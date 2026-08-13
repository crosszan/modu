package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openmodu/modu/pkg/coding_agent/sessionipc"
)

func TestServeSessionAppServerOwnsOneSocketAndCleansUp(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), ".modu")
	runtimeDir, err := os.MkdirTemp("/tmp", "modu-app-server-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveSessionAppServer(ctx, agentDir, runtimeDir) }()

	waitForAppServerTest(t, time.Second, func() bool {
		probeCtx, probeCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer probeCancel()
		_, pidErr := readAppServerPID(runtimeDir)
		return sessionipc.Probe(probeCtx, runtimeDir) == nil && pidErr == nil
	})
	if _, err := readAppServerPID(runtimeDir); err != nil {
		t.Fatalf("readAppServerPID() error = %v", err)
	}
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	sockets := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeSocket != 0 {
			sockets++
		}
	}
	if sockets != 1 {
		t.Fatalf("socket count = %d, want one", sockets)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveSessionAppServer() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("app-server did not stop")
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "ipc.sock")); !os.IsNotExist(err) {
		t.Fatalf("socket remains after shutdown: %v", err)
	}
	if _, err := os.Stat(appServerPIDPath(runtimeDir)); !os.IsNotExist(err) {
		t.Fatalf("pid file remains after shutdown: %v", err)
	}
}

func TestRunAppServerStatusReportsStoppedWithoutStartingDaemon(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), ".modu")
	var stdout testBuffer
	if err := runAppServerCommand([]string{"status", "--agent-dir", agentDir}, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "app-server stopped\n" {
		t.Fatalf("status output = %q", stdout.String())
	}
}

type testBuffer struct{ data []byte }

func (b *testBuffer) Write(data []byte) (int, error) {
	b.data = append(b.data, data...)
	return len(data), nil
}

func (b *testBuffer) String() string { return string(b.data) }

func waitForAppServerTest(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
