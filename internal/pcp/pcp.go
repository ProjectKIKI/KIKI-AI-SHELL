package pcp

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Client executes PCP CLI tools (pmrep/pmval) to query local or remote pmcd.
// Remote requires pmcd running on the target host and network access (default port 44321).
//
// We keep this intentionally simple (no extra deps) so it works in minimal lab VMs.

type Client struct {
	Host string // "local" or hostname/IP
}

func New(host string) *Client {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "local"
	}
	return &Client{Host: host}
}

func (c *Client) SetHost(host string) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "local"
	}
	c.Host = host
}

func (c *Client) HostLabel() string {
	if c == nil {
		return "local"
	}
	if strings.TrimSpace(c.Host) == "" {
		return "local"
	}
	return c.Host
}

// Display returns a short label for UI header.
func (c *Client) Display() string { return c.HostLabel() }

func (c *Client) baseArgs() []string {
	// PCP tools typically accept -h <host> to query remote.
	if c == nil {
		return nil
	}
	h := strings.TrimSpace(c.Host)
	if h == "" || strings.EqualFold(h, "local") || h == "127.0.0.1" || h == "localhost" {
		return nil
	}
	return []string{"-h", h}
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		es := strings.TrimSpace(errb.String())
		if es == "" {
			es = err.Error()
		}
		return strings.TrimSpace(out.String()), fmt.Errorf("%s: %s", name, es)
	}
	return strings.TrimSpace(out.String()), nil
}

// Raw runs pmrep for the given metrics and returns raw text.
func (c *Client) Raw(metrics []string, samples int, interval time.Duration) (string, error) {
	if len(metrics) == 0 {
		return "", errors.New("no metrics")
	}
	if !commandExists("pmrep") {
		return "", errors.New("pmrep not found (install pcp package)")
	}
	if samples <= 0 {
		samples = 1
	}
	if interval <= 0 {
		interval = 1 * time.Second
	}
	args := []string{}
	args = append(args, c.baseArgs()...)
	args = append(args,
		"-s", fmt.Sprintf("%d", samples),
		"-t", fmt.Sprintf("%ds", int(interval.Seconds())),
		"-o", strings.Join(metrics, ","),
	)
	return run("pmrep", args...)
}

// Quick tries to present a short, human friendly snapshot.
// We keep the metric list conservative to avoid instance-heavy metrics.
func (c *Client) Quick() (string, error) {
	if !commandExists("pmrep") {
		return "", errors.New("pmrep not found (install pcp package)")
	}
	metrics := []string{
		"kernel.all.load",           // 1/5/15 load averages
		"mem.util.used",             // used memory
		"mem.util.free",             // free memory
		"kernel.all.cpu.user",       // cpu user
		"kernel.all.cpu.sys",        // cpu sys
		"kernel.all.cpu.idle",       // cpu idle
		"network.interface.in.bytes",  // may be instance-heavy but widely present
		"network.interface.out.bytes", // may be instance-heavy
	}
	// Use 1 sample, 1 second. pmrep prints a small table.
	out, err := c.Raw(metrics, 1, 1*time.Second)
	if err != nil {
		// fallback to pmval for load if pmrep fails
		if commandExists("pmval") {
			args := append([]string{}, c.baseArgs()...)
			args = append(args, "-s", "1", "kernel.all.load")
			v, e2 := run("pmval", args...)
			if e2 == nil {
				return v, nil
			}
		}
		return "", err
	}
	return out, nil
}

