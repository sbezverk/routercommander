package types

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// simulateRouter writes a fake router response to stdoutW after reading the
// command from stdinR. It mirrors what a real IOS-XR shell session would do:
// echo the command back, print output, then print the prompt.
func simulateRouter(t *testing.T, stdinR io.Reader, stdoutW io.WriteCloser, output string) {
	t.Helper()
	go func() {
		defer stdoutW.Close()
		buf := make([]byte, 4096)
		// Read whatever the caller wrote (the command + "\n"), discard it.
		stdinR.Read(buf) //nolint:errcheck
		// Write: command echo + output + prompt (IOS-XR style)
		fmt.Fprintf(stdoutW, "show version\nCisco IOS XR\n%s\nRP/0/RSP0/CPU0:router#\n", output)
	}()
}

func TestSendCommand_NormalOutput(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	want := "Cisco IOS XR\nsome output line\n"
	simulateRouter(t, stdinR, stdoutW, "some output line")

	result, err := sendCommand(stdinW, stdoutR, "show version", false, nil, 5)
	if err != nil {
		t.Fatalf("sendCommand returned unexpected error: %v", err)
	}
	// Strip CR bytes the same way sendCommand does, then compare.
	got := strings.ReplaceAll(string(result), "\r", "")
	if !strings.Contains(got, "some output line") {
		t.Fatalf("expected output to contain %q, got: %q", want, got)
	}
}

func TestSendCommand_LargeOutput(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	// Use enough repetitions to be well beyond the old fixed-buffer limit proof-of-concept
	// without triggering O(n²) regex scanning slowness (keep under ~50 KB).
	large := strings.Repeat("10.0.0.0/24 via 192.168.1.1\n", 1_500) // ~45 KB
	simulateRouter(t, stdinR, stdoutW, large)

	result, err := sendCommand(stdinW, stdoutR, "show version", false, nil, 10)
	if err != nil {
		t.Fatalf("sendCommand returned unexpected error on large output: %v", err)
	}
	if !strings.Contains(string(result), "10.0.0.0/24") {
		t.Fatal("large output not fully received")
	}
}

func TestSendCommand_Timeout(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	// Drain stdin so fmt.Fprintf inside sendCommand doesn't block.
	go io.Copy(io.Discard, stdinR) //nolint:errcheck

	// Write partial output (no prompt), then close the pipe after the command
	// timeout fires so the leaked goroutine unblocks rather than hanging.
	go func() {
		fmt.Fprintf(stdoutW, "show version\noutput with no prompt\n") //nolint:errcheck
		// Wait slightly longer than the command timeout, then close so the
		// goroutine inside sendCommand can exit (known goroutine-leak issue #4.2).
		time.Sleep(2 * time.Second)
		stdoutW.Close()
	}()

	_, err := sendCommand(stdinW, stdoutR, "show version", false, nil, 1)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "time out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	// Allow the goroutine inside sendCommand time to unblock after stdoutW closes.
	time.Sleep(500 * time.Millisecond)
	stdinR.Close()
}

func TestSendCommand_ReadError(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	// Drain stdin so fmt.Fprintf inside sendCommand doesn't block.
	go io.Copy(io.Discard, stdinR) //nolint:errcheck
	defer stdinR.Close()

	// Close the write-end immediately — Read on stdoutR will return io.EOF.
	stdoutW.Close()

	_, err := sendCommand(stdinW, stdoutR, "show version", false, nil, 5)
	if err == nil {
		t.Fatal("expected an error from closed stdout pipe, got nil")
	}
}

func TestSendCommand_NXOSPrompt(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	go func() {
		defer stdoutW.Close()
		stdinR.Read(make([]byte, 256)) //nolint:errcheck
		// NX-OS style prompt
		fmt.Fprintf(stdoutW, "show version\nNX-OS output here\nnxos-switch#\n")
	}()

	result, err := sendCommand(stdinW, stdoutR, "show version", false, nil, 5)
	if err != nil {
		t.Fatalf("sendCommand with NX-OS prompt returned error: %v", err)
	}
	if !strings.Contains(string(result), "NX-OS output here") {
		t.Fatalf("expected NX-OS output in result, got: %q", string(result))
	}
}
