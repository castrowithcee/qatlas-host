package hostchange

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// MaxShellOutput bounds the output of one free command that is kept; the rest is discarded.
const MaxShellOutput = 64 << 10

// DefaultShellTimeout is the time one free command may run.
const DefaultShellTimeout = 5 * time.Minute

// shellWaitDelay bounds how long the run waits for the output after the shell has ended, for a process that
// keeps the pipe open.
const shellWaitDelay = 2 * time.Second

// ShellResult is what one free command did. Ran is false for a command that never started because an earlier
// one failed. ExitCode is -1 when the shell did not end with an exit code, as after a timeout.
type ShellResult struct {
	Command   string `json:"command"`
	Ran       bool   `json:"ran"`
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out"`
}

// Succeeded reports whether the command ran and exited with 0.
func (r ShellResult) Succeeded() bool { return r.Ran && r.ExitCode == 0 && !r.TimedOut }

// ShellExecutor runs one free command; tests replace it.
type ShellExecutor interface {
	RunShell(ctx context.Context, command string, timeout time.Duration) ShellResult
}

// shellEnv is all a free command gets from its environment: nothing comes from the caller.
func shellEnv() []string {
	return []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "HOME=/root"}
}

// SystemShell runs a command with /bin/sh -c as the current user.
type SystemShell struct{}

// RunShell starts the command in a process group of its own with a fixed environment and closed standard input.
// When the time is up or ctx ends, the whole group is killed.
func (SystemShell) RunShell(ctx context.Context, command string, timeout time.Duration) ShellResult {
	res := ShellResult{Command: command, Ran: true, ExitCode: -1}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		res.Output = "cannot open " + os.DevNull + ": " + err.Error()
		return res
	}
	defer devNull.Close()
	out := &limitedBuffer{limit: MaxShellOutput}
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = shellEnv(), devNull, out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = shellWaitDelay
	if err := cmd.Start(); err != nil {
		res.Output = "cannot start the shell: " + err.Error()
		return res
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-timer.C:
		res.TimedOut = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		waitErr = <-done
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		waitErr = <-done
	}
	if waitErr == nil {
		res.ExitCode = 0
	} else if code := cmd.ProcessState.ExitCode(); code >= 0 {
		res.ExitCode = code
	}
	if res.TimedOut {
		res.ExitCode = -1
	}
	res.Output, res.Truncated = out.result()
	return res
}

// limitedBuffer keeps the first limit bytes and counts the rest. It never fails a write, so the process is not
// blocked by a full pipe.
type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.limit - b.buf.Len(); len(p) <= room {
		b.buf.Write(p)
	} else {
		b.buf.Write(p[:room])
		b.truncated = true
	}
	return len(p), nil
}

func (b *limitedBuffer) result() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.ToValidUTF8(b.buf.String(), "�"), b.truncated
}
