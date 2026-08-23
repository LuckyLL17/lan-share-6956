package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestGracefulShutdownLifecycle(t *testing.T) {
	// The process path covers main.go, internal/service/discover_service.go,
	// and internal/network/udp.go.
	httpPort := freePort(t)
	udpPort := freePort(t)
	dataDir := t.TempDir()
	binary := filepath.Join(t.TempDir(), "lan-share")

	build := exec.Command("go", "build", "-o", binary, ".")
	build.Env = append(os.Environ(), "GOCACHE=/tmp/lan-share-gocache")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build service: %v\n%s", err, output)
	}

	cmd := exec.Command(binary, "-port", strconv.Itoa(httpPort))
	cmd.Env = append(os.Environ(),
		"LANSHARE_NETWORK_UDP_DISCOVER_PORT="+strconv.Itoa(udpPort),
		"LANSHARE_STORAGE_DATA_DIR="+dataDir,
	)
	output := &strings.Builder{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start service: %v", err)
	}

	healthURL := "http://127.0.0.1:" + strconv.Itoa(httpPort) + "/api/v1/health"
	client := &http.Client{Timeout: 250 * time.Millisecond}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if time.Now().After(deadline) {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		t.Fatalf("service did not become healthy:\n%s", output.String())
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal service: %v", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("service exited abnormally: %v\n%s", err, output.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-wait
		t.Fatal("service did not finish graceful shutdown")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
