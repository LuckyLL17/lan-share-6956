package network

import (
	"testing"
	"time"
)

// TestUDPDiscovery_CloseWithoutListen 验证未启动场景：Close 不应永久阻塞。
// 回归 bug：构造后直接 Close 会卡在 <-u.done，因为 readLoop 从未启动。
func TestUDPDiscovery_CloseWithoutListen(t *testing.T) {
	u := NewUDPDiscovery(0, "test")
	done := make(chan struct{})
	go func() {
		u.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("UDP close hung when Listen was never called")
	}
}

// TestUDPDiscovery_CloseAfterListenFailure 验证启动失败清理场景。
// Listen 失败（端口已被占用）后 Close 不应永久阻塞。
func TestUDPDiscovery_CloseAfterListenFailure(t *testing.T) {
	// 先占用一个端口，使第二次 Listen 失败。
	occupier := NewUDPDiscovery(0, "occupier")
	if err := occupier.Listen(); err != nil {
		t.Fatalf("occupier listen: %v", err)
	}
	defer occupier.Close()
	port := occupier.LocalPort()
	// 在已占用端口上再 Listen 必然失败。
	u := NewUDPDiscovery(port, "test")
	if err := u.Listen(); err == nil {
		u.Close()
		t.Fatalf("expected listen failure on occupied port %d", port)
	}
	done := make(chan struct{})
	go func() {
		u.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("UDP close hung during startup cleanup")
	}
}

// TestUDPDiscovery_NormalLifecycle 验证正常运行场景：Listen 成功后 Close 能在有限时间结束。
func TestUDPDiscovery_NormalLifecycle(t *testing.T) {
	u := NewUDPDiscovery(0, "test")
	if err := u.Listen(); err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		u.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("UDP close hung after successful Listen")
	}
}

// TestUDPDiscovery_CloseIdempotent 验证 Close 多次调用安全。
func TestUDPDiscovery_CloseIdempotent(t *testing.T) {
	u := NewUDPDiscovery(0, "test")
	if err := u.Listen(); err != nil {
		t.Fatalf("listen: %v", err)
	}
	for i := 0; i < 3; i++ {
		u.Close()
	}
}

