package ui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

type winsize struct {
	Row, Col, Xpixel, Ypixel uint16
}

// TermCols returns terminal columns with sane fallbacks.
func TermCols() int {
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdout), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if errno != 0 || ws.Col == 0 {
		if v := strings.TrimSpace(os.Getenv("COLUMNS")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
		return 80
	}
	return int(ws.Col)
}

// TermRows returns terminal rows with sane fallbacks.
func TermRows() int {
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(syscall.Stdout), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if errno != 0 || ws.Row == 0 {
		if v := strings.TrimSpace(os.Getenv("LINES")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
		}
		return 24
	}
	return int(ws.Row)
}

// ElideMiddle shortens a string by replacing the middle with an ellipsis.
func ElideMiddle(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	if max <= 3 {
		return string(r[:max])
	}
	head := (max - 1) / 2
	tail := max - head - 1
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

// --- Cursor + scroll region helpers ---

// SaveCursor saves current cursor position.
func SaveCursor() { fmt.Print("\033[s") }

// RestoreCursor restores the last saved cursor position.
func RestoreCursor() { fmt.Print("\033[u") }

// CursorHome moves cursor to 1,1.
func CursorHome() { fmt.Print("\033[H") }

// MoveCursor moves cursor to (row,col) where row/col are 1-based.
func MoveCursor(row, col int) {
	if row < 1 {
		row = 1
	}
	if col < 1 {
		col = 1
	}
	fmt.Printf("\033[%d;%dH", row, col)
}

// CursorAt is an alias of MoveCursor.
func CursorAt(row, col int) { MoveCursor(row, col) }

// SetScrollRegion sets the terminal scroll region (DECSTBM).
func SetScrollRegion(top, bottom int) {
	if top < 1 {
		top = 1
	}
	if bottom < top {
		bottom = top
	}
	fmt.Printf("\033[%d;%dr", top, bottom)
}

// ResetScrollRegion resets the terminal scroll region.
func ResetScrollRegion() { fmt.Print("\033[r") }

// ClearLine clears the current line.
func ClearLine() { fmt.Print("\033[2K") }
