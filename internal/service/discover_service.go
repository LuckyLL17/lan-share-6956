package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"lan-share/config"
	"lan-share/internal/model"
	"lan-share/internal/network"
	"lan-share/internal/repository"
)

// DiscoverService 负责局域网设备发现与设备列表维护。
// 它协调 UDP 广播/监听 与 持久化层，是设备发现功能的核心。
type DiscoverService struct {
	cfg     *config.Config
	udp     *network.UDPDiscovery
	devRepo *repository.DeviceRepository

	mu      sync.RWMutex
	cache   map[string]model.Device // ID -> 设备快照
	msgRepo *repository.MessageRepository

	stopCh chan struct{}
	done   chan struct{}
}

// NewDiscoverService 构造发现服务。
func NewDiscoverService(cfg *config.Config, udp *network.UDPDiscovery, devRepo *repository.DeviceRepository, msgRepo *repository.MessageRepository) *DiscoverService {
	return &DiscoverService{
		cfg:     cfg,
		udp:     udp,
		devRepo: devRepo,
		msgRepo: msgRepo,
		cache:   make(map[string]model.Device),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start 启动发现服务：开始监听 + 周期广播。
func (s *DiscoverService) Start(ctx context.Context) error {
	// 注册 UDP 包回调
	s.udp.OnPacket(s.handleDiscoveryPacket)
	s.udp.OnMessage(s.handleMessagePacket)

	if err := s.udp.Listen(); err != nil {
		return fmt.Errorf("udp listen: %w", err)
	}
	go s.broadcastLoop(ctx)
	go s.staleLoop(ctx)
	log.Printf("[discover] started, device=%s udp_port=%d", s.cfg.Device.Name, s.cfg.Network.UDPDiscoverPort)
	return nil
}

// Stop 停止发现服务。
func (s *DiscoverService) Stop() {
	close(s.stopCh)
	<-s.done
	s.udp.Close()
	log.Println("[discover] stopped")
}

// DiscoverNow 立即触发一次主动广播。
func (s *DiscoverService) DiscoverNow(ctx context.Context) error {
	pkt := s.buildSelfPacket()
	if err := s.udp.Broadcast(pkt); err != nil {
		return err
	}
	// 主动探测后清理一次离线设备
	if _, err := s.devRepo.MarkStale(ctx, time.Duration(s.cfg.Network.DeviceTTL)*time.Second); err != nil {
		log.Printf("[discover] mark stale: %v", err)
	}
	s.refreshCache(ctx)
	return nil
}

// SendMessage 向局域网发送一条消息。
// toIP 为空表示广播给所有设备。
func (s *DiscoverService) SendMessage(ctx context.Context, toIP, content string) error {
	pkt := network.MessagePacket{
		FromIP:   localIP(),
		FromName: s.cfg.Device.Name,
		ToIP:     toIP,
		Content:  content,
	}
	if err := s.udp.SendMessage(pkt); err != nil {
		log.Printf("[discover] send message: %v", err)
		return nil
	}
	return nil
}

// ListDevices 返回当前已知设备列表（内存缓存优先）。
func (s *DiscoverService) ListDevices(ctx context.Context) ([]model.Device, error) {
	s.mu.RLock()
	if len(s.cache) > 0 {
		out := make([]model.Device, 0, len(s.cache))
		for _, d := range s.cache {
			d.CalcOnlineDuration(time.Now())
			out = append(out, d)
		}
		s.mu.RUnlock()
		return out, nil
	}
	s.mu.RUnlock()
	// 缓存为空则回源
	return s.devRepo.ListAll(ctx)
}

// GetDevice 查询单个设备。
func (s *DiscoverService) GetDevice(ctx context.Context, id string) (*model.Device, error) {
	s.mu.RLock()
	if d, ok := s.cache[id]; ok {
		d.CalcOnlineDuration(time.Now())
		s.mu.RUnlock()
		return &d, nil
	}
	s.mu.RUnlock()
	return s.devRepo.Get(ctx, id)
}

// handleDiscoveryPacket 处理收到的 UDP 发现包。
func (s *DiscoverService) handleDiscoveryPacket(pkt network.DiscoverPacket) {
	if pkt.Name == s.cfg.Device.Name && pkt.IP == localIP() {
		// 忽略自己
		return
	}
	dev := model.Device{
		ID:     model.DeviceIDFromIPName(pkt.IP, pkt.Name),
		Name:   pkt.Name,
		IP:     pkt.IP,
		Port:   pkt.Port,
		Status: model.DeviceStatusOnline,
		OS:     pkt.OS,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.devRepo.Upsert(ctx, dev); err != nil {
		log.Printf("[discover] upsert device %s: %v", dev.ID, err)
		return
	}
	dev.CalcOnlineDuration(time.Now())
	s.mu.Lock()
	s.cache[dev.ID] = dev
	s.mu.Unlock()
}

// handleMessagePacket 处理收到的消息包。
func (s *DiscoverService) handleMessagePacket(pkt network.MessagePacket) {
	if pkt.FromIP == localIP() && pkt.FromName == s.cfg.Device.Name {
		return
	}
	m := &model.Message{
		FromIP:   pkt.FromIP,
		FromName: pkt.FromName,
		ToIP:     pkt.ToIP,
		Content:  pkt.Content,
	}
	if err := m.Validate(); err != nil {
		log.Printf("[discover] invalid message: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.msgRepo.Create(ctx, m); err != nil {
		log.Printf("[discover] save message: %v", err)
	}
}

// broadcastLoop 周期性广播本机存在。
func (s *DiscoverService) broadcastLoop(ctx context.Context) {
	defer close(s.done)
	interval := time.Duration(s.cfg.Network.DiscoverInterval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	pkt := s.buildSelfPacket()
	// 启动即广播一次
	if err := s.udp.Broadcast(pkt); err != nil {
		log.Printf("[discover] initial broadcast: %v", err)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-t.C:
			if err := s.udp.Broadcast(s.buildSelfPacket()); err != nil {
				log.Printf("[discover] broadcast: %v", err)
			}
		}
	}
}

// staleLoop 周期性把过期设备标记为离线。
func (s *DiscoverService) staleLoop(ctx context.Context) {
	ttl := time.Duration(s.cfg.Network.DeviceTTL) * time.Second
	if ttl < 5*time.Second {
		ttl = 15 * time.Second
	}
	t := time.NewTicker(ttl)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-t.C:
			ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			n, err := s.devRepo.MarkStale(ctx2, ttl)
			cancel()
			if err != nil {
				log.Printf("[discover] mark stale: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("[discover] %d device(s) marked offline", n)
				s.refreshCache(context.Background())
			}
		}
	}
}

// refreshCache 从持久化层刷新内存缓存。
func (s *DiscoverService) refreshCache(ctx context.Context) {
	all, err := s.devRepo.ListAll(ctx)
	if err != nil {
		log.Printf("[discover] refresh cache: %v", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = make(map[string]model.Device, len(all))
	for _, d := range all {
		s.cache[d.ID] = d
	}
}

// buildSelfPacket 构造本机的发现广播包。
func (s *DiscoverService) buildSelfPacket() network.DiscoverPacket {
	return network.DiscoverPacket{
		Name: s.cfg.Device.Name,
		IP:   localIP(),
		Port: s.cfg.Network.FileServerPort,
		OS:   goosName(),
	}
}

// localIP 返回本机首选局域网 IP。
// 在 internal/network 包中维护，这里通过别名访问。
func localIP() string {
	if ip, err := network.LocalIP(); err == nil && ip != "" {
		return ip
	}
	return "127.0.0.1"
}

// goosName 返回简短 OS 名。
func goosName() string {
	return network.GoOS()
}
