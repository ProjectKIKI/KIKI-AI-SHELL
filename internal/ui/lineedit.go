package ui

import (
    "errors"
    "fmt"
    "io"
    "os"
    "sort"
    "strings"
	"syscall"
    "unicode/utf8"
	"unsafe"
)

// ReadLineRaw is a tiny line editor for the interactive REPL.
//
// Features:
//   - basic input / backspace
//   - TAB completion (prints candidates, or auto-completes when unique)
//
// It intentionally stays dependency-free (no external readline library).
func ReadLineRaw(prompt string, complete func(line string) []string) (string, error) {
    if prompt != "" {
        fmt.Fprint(os.Stdout, prompt)
    }

	fd := int(os.Stdin.Fd())
	oldState, err := makeRaw(fd)
    if err != nil {
        return "", err
    }
	defer func() { _ = restore(fd, oldState) }()

    var buf []rune
    b := make([]byte, 1)
    for {
        n, err := os.Stdin.Read(b)
        if err != nil {
            return "", err
        }
        if n == 0 {
            continue
        }
        c := b[0]

        switch c {
        case '\r', '\n':
			// Move to the next line and return to column 0.
			fmt.Fprint(os.Stdout, "\r\n")
            return strings.TrimSpace(string(buf)), nil
        case 3: // Ctrl-C
			fmt.Fprint(os.Stdout, "\r\n")
            return "", errors.New("interrupted")
        case 4: // Ctrl-D
            if len(buf) == 0 {
				fmt.Fprint(os.Stdout, "\r\n")
				return "", io.EOF
            }
        case 127, 8: // Backspace
            if len(buf) > 0 {
                buf = buf[:len(buf)-1]
                // move left, clear char, move left
                fmt.Fprint(os.Stdout, "\b \b")
            }
        case '\t':
            if complete == nil {
                continue
            }
            cur := string(buf)
            cands := complete(cur)
            if len(cands) == 0 {
                continue
            }
            sort.Strings(cands)
            if len(cands) == 1 {
                // replace whole line with candidate
                // clear current line: backspace all
                for range buf {
                    fmt.Fprint(os.Stdout, "\b \b")
                }
                buf = []rune(cands[0])
                fmt.Fprint(os.Stdout, cands[0])
                continue
            }
            // multiple candidates: print list then redraw prompt+buf
            fmt.Fprint(os.Stdout, "\n")
            cols := TermCols()
            if cols <= 0 {
                cols = 80
            }
            // print candidates in wrapped columns
            maxw := 0
            for _, s := range cands {
                if w := len([]rune(s)); w > maxw {
                    maxw = w
                }
            }
            if maxw < 8 {
                maxw = 8
            }
            cell := maxw + 2
            perRow := cols / cell
            if perRow < 1 {
                perRow = 1
            }
			for i, s := range cands {
				fmt.Fprint(os.Stdout, padRight(s, cell))
                if (i+1)%perRow == 0 {
                    fmt.Fprint(os.Stdout, "\n")
                }
            }
            if len(cands)%perRow != 0 {
                fmt.Fprint(os.Stdout, "\n")
            }
            if prompt != "" {
                fmt.Fprint(os.Stdout, prompt)
            }
            fmt.Fprint(os.Stdout, string(buf))
        default:
            // printable ASCII + UTF-8 bytes: we accept bytes and append as rune when possible.
            if c < 32 {
                continue
            }
            // Decode full UTF-8 rune.
            rb := []byte{c}
            if c >= 0x80 {
                // Determine expected length from first byte.
                need := 0
                switch {
                case c&0xE0 == 0xC0:
                    need = 2
                case c&0xF0 == 0xE0:
                    need = 3
                case c&0xF8 == 0xF0:
                    need = 4
                default:
                    need = 1
                }
                for len(rb) < need {
                    nn, e := os.Stdin.Read(b)
                    if e != nil {
                        return "", e
                    }
                    if nn == 0 {
                        continue
                    }
                    rb = append(rb, b[0])
                }
            }
            r, _ := utf8.DecodeRune(rb)
            if r == utf8.RuneError {
                continue
            }
            buf = append(buf, r)
            fmt.Fprint(os.Stdout, string(r))
        }
    }
}

// NOTE: On empty line Ctrl-D returns io.EOF.

// --- minimal raw mode (linux/unix) ---
// We implement the small subset we need to read keystrokes (TAB, backspace) without
// pulling external modules.

func makeRaw(fd int) (*syscall.Termios, error) {
    old, err := getTermios(fd)
    if err != nil {
        return nil, err
    }
    raw := *old
    // from x/term.MakeRaw (simplified)
    raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
    raw.Oflag &^= syscall.OPOST
	// IEXTEN enables implementation-defined input processing (like ^V).
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
    raw.Cflag &^= syscall.CSIZE | syscall.PARENB
    raw.Cflag |= syscall.CS8
    raw.Cc[syscall.VMIN] = 1
    raw.Cc[syscall.VTIME] = 0
    if err := setTermios(fd, &raw); err != nil {
        return nil, err
    }
    return old, nil
}

func restore(fd int, st *syscall.Termios) error {
    if st == nil {
        return nil
    }
    return setTermios(fd, st)
}

func getTermios(fd int) (*syscall.Termios, error) {
    t := &syscall.Termios{}
    if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(t)), 0, 0, 0); errno != 0 {
        return nil, errno
    }
    return t, nil
}

func setTermios(fd int, t *syscall.Termios) error {
    if _, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(t)), 0, 0, 0); errno != 0 {
        return errno
    }
    return nil
}
