// Package network 封装局域网底层通信：UDP 发现广播与消息、HTTP 文件传输。
package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"sync"
	"time"
)

// DiscoverPacket UDP 设备发现广播包
type DiscoverPacket struct {
	Type string `json:"t"` // "discover"
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
	OS   string `json:"os,omitempty"`
}

// MessagePacket UDP 消息包
type MessagePacket struct {
	Type     string `json:"t"` // "msg"
	FromIP   string `json:"from_ip"`
	FromName string `json:"from_name"`
	ToIP     string `json:"to_ip"`
	Content  string `json:"content"`
}

// packetEnvelope 区分包类型的信封
type packetEnvelope struct {
	Type string `json:"t"`
}

// DiscoverCallback 收到发现包的回调
type DiscoverCallback func(DiscoverPacket)

// MessageCallback 收到消息包的回调
type MessageCallback func(MessagePacket)

// UDPDiscovery 负责 UDP 发现广播与接收。
type UDPDiscovery struct {
	port    int
	conn    *net.UDPConn
	selfIP  string
	selfName string

	mu        sync.RWMutex
	discoverCb DiscoverCallback
	messageCb  MessageCallback

	stopOnce sync.Once
	stopCh   chan struct{}
	done     chan struct{}
	stateMu  sync.Mutex
	started  bool
	closed   bool
}

// NewUDPDiscovery 构造发现实例。
func NewUDPDiscovery(port int, selfName string) *UDPDiscovery {
	return &UDPDiscovery{
		port:     port,
		selfName: selfName,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
		started:  true,
	}
}

// OnPacket 设置发现包回调
func (u *UDPDiscovery) OnPacket(cb DiscoverCallback) {
	u.mu.Lock()
	u.discoverCb = cb
	u.mu.Unlock()
}

// OnMessage 设置消息包回调
func (u *UDPDiscovery) OnMessage(cb MessageCallback) {
	u.mu.Lock()
	u.messageCb = cb
	u.mu.Unlock()
}

// Listen 开始监听 UDP 端口。
// 多次调用会复用同一连接，避免重复绑定。
func (u *UDPDiscovery) Listen() error {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", u.port))
	if err != nil {
		return fmt.Errorf("resolve udp: %w", err)
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("listen udp %d: %w", u.port, err)
	}
	conn.SetReadBuffer(64 * 1024)
	u.conn = conn
	u.stateMu.Lock()
	u.started = true
	u.closed = false
	u.stateMu.Unlock()
	u.selfIP, _ = LocalIP()
	go u.readLoop()
	log.Printf("[udp] listening on %d", u.port)
	return nil
}

// readLoop 接收并分发 UDP 包。
func (u *UDPDiscovery) readLoop() {
	defer close(u.done)
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-u.stopCh:
			return
		default:
		}
		n, src, err := u.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-u.stopCh:
				return
			default:
			}
			log.Printf("[udp] read: %v", err)
			continue
		}
		u.dispatch(buf[:n], src)
	}
}

// dispatch 根据包信封分发到对应回调。
func (u *UDPDiscovery) dispatch(data []byte, src *net.UDPAddr) {
	var env packetEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	switch env.Type {
	case "discover":
		var p DiscoverPacket
		if err := json.Unmarshal(data, &p); err != nil {
			return
		}
		// 若包内未带 IP，使用来源 IP
		if p.IP == "" {
			p.IP = src.IP.String()
		}
		u.mu.RLock()
		cb := u.discoverCb
		u.mu.RUnlock()
		if cb != nil {
			cb(p)
		}
	case "msg":
		var p MessagePacket
		if err := json.Unmarshal(data, &p); err != nil {
			return
		}
		if p.FromIP == "" {
			p.FromIP = src.IP.String()
		}
		u.mu.RLock()
		cb := u.messageCb
		u.mu.RUnlock()
		if cb != nil {
			cb(p)
		}
	}
}

// Broadcast 向局域网广播设备发现包。
func (u *UDPDiscovery) Broadcast(p DiscoverPacket) error {
	if u.conn == nil {
		return errors.New("udp not listening")
	}
	p.Type = "discover"
	if p.IP == "" {
		p.IP = u.selfIP
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	bcast, err := broadcastAddr(u.port)
	if err != nil {
		return err
	}
	if _, err := u.conn.WriteToUDP(data, bcast); err != nil {
		return err
	}
	return nil
}

// SendMessage 广播或定向发送消息包。
func (u *UDPDiscovery) SendMessage(p MessagePacket) error {
	if u.conn == nil {
		return errors.New("udp not listening")
	}
	p.Type = "msg"
	if p.FromIP == "" {
		p.FromIP = u.selfIP
	}
	if p.FromName == "" {
		p.FromName = u.selfName
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	// 定向单播
	if p.ToIP != "" && p.ToIP != "255.255.255.255" {
		addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", p.ToIP, u.port))
		if err != nil {
			return err
		}
		_, err = u.conn.WriteToUDP(data, addr)
		return err
	}
	// 广播
	bcast, err := broadcastAddr(u.port)
	if err != nil {
		return err
	}
	_, err = u.conn.WriteToUDP(data, bcast)
	return err
}

// Close 关闭监听。
func (u *UDPDiscovery) Close() {
	u.stopOnce.Do(func() {
		u.stateMu.Lock()
		u.closed = true
		conn := u.conn
		u.stateMu.Unlock()
		close(u.stopCh)
		if conn != nil {
			_ = conn.Close()
		}
		<-u.done
	})
}

// broadcastAddr 返回本机所在子网的广播地址。
// 简化实现：对 192.168.x.x/24 等常见掩码返回 x.x.x.255。
func broadcastAddr(port int) (*net.UDPAddr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := parseAddr(a)
			if !ok {
				continue
			}
			if ipn.IP.To4() == nil {
				continue
			}
			bcast := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				bcast[i] = ipn.IP.To4()[i] | ^ipn.Mask[i]
			}
			return net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", bcast.String(), port))
		}
	}
	// 退化：全 1 广播
	return net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", port))
}

// parseAddr 解析 Addr 为 IPNet
func parseAddr(a net.Addr) (*net.IPNet, bool) {
	switch v := a.(type) {
	case *net.IPNet:
		return v, true
	case *net.IPAddr:
		ipn := &net.IPNet{IP: v.IP, Mask: v.IP.DefaultMask()}
		return ipn, true
	}
	return nil, false
}

// LocalIP 返回本机首选 IPv4 局域网地址。
func LocalIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := parseAddr(a)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil {
				continue
			}
			// 排除链路本地
			if ip4[0] == 169 && ip4[1] == 254 {
				continue
			}
			return ip4.String(), nil
		}
	}
	// 退化
	return "127.0.0.1", nil
}

// GoOS 返回当前操作系统名
func GoOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return runtime.GOOS
}

// Hostname 返回主机名，失败时回退。
func Hostname() string {
	h, err := os.Hostname()
	if err == nil && h != "" {
		return h
	}
	return "lan-share"
}

// SetReadTimeout 设置读超时（仅用于同步读取，当前 readLoop 不使用）。
func SetReadTimeout(conn *net.UDPConn, d time.Duration) {
	if conn == nil {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(d))
}
