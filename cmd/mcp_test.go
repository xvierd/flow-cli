package cmd

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestMCPCmd_StdoutCarriesOnlyJSONRPC guards the stdio transport: the client
// reads stdout as a stream of JSON-RPC messages, so the human banner must not
// be written there (it broke the initialize handshake of strict clients).
func TestMCPCmd_StdoutCarriesOnlyJSONRPC(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = inR, outW, errW
	t.Cleanup(func() { os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr })
	// ServeStdio registers its own SIGINT/SIGTERM handler; undo it.
	t.Cleanup(func() { signal.Reset(syscall.SIGINT, syscall.SIGTERM) })

	stderrDone := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(errR)
		stderrDone <- string(b)
	}()

	runDone := make(chan error, 1)
	go func() { runDone <- mcpCmd.RunE(mcpCmd, nil) }()

	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}` + "\n"
	if _, err := inW.WriteString(initReq); err != nil {
		t.Fatal(err)
	}

	// The very first line on stdout must be the initialize response.
	lineCh := make(chan string, 1)
	reader := bufio.NewReader(outR)
	go func() {
		line, _ := reader.ReadString('\n')
		lineCh <- line
	}()
	var first string
	select {
	case first = <-lineCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for initialize response")
	}

	// EOF on stdin ends the server.
	_ = inW.Close()
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("mcp command returned %v on stdin EOF, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mcp command did not return after stdin EOF")
	}
	_ = outW.Close()
	_ = errW.Close()
	rest, _ := io.ReadAll(reader)
	stderr := <-stderrDone

	lines := []string{first}
	for _, l := range strings.Split(string(rest), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	for i, l := range lines {
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(l), &msg); err != nil {
			t.Fatalf("stdout line %d is not JSON-RPC: %q (%v)", i, l, err)
		}
		if msg["jsonrpc"] != "2.0" {
			t.Errorf("stdout line %d is not a JSON-RPC 2.0 message: %q", i, l)
		}
	}
	if !strings.Contains(stderr, "MCP") {
		t.Errorf("expected the startup banner on stderr, got %q", stderr)
	}
}
