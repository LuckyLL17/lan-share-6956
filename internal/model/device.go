package model

import "time"

// DeviceStatus 设备在线状态枚举
type DeviceStatus string

const (
	DeviceStatusOnline  DeviceStatus = "online"
	DeviceStatusOffline DeviceStatus = "offline"
)

// Device 描述一台局域网内被发现/记录的设备。
// 它是设备发现与设备列表展示的核心数据结构。
type Device struct {
	// ID 设备唯一标识（基于 IP+名称生成的稳定 hash）
	ID string `json:"id"`
	// Name 设备显示名
	Name string `json:"name"`
	// IP 设备的局域网 IPv4 地址
	IP string `json:"ip"`
	// Port 设备暴露 HTTP 文件服务的端口
	Port int `json:"port"`
	// Status 当前在线状态
	Status DeviceStatus `json:"status"`
	// FirstSeen 首次发现时间
	FirstSeen time.Time `json:"first_seen"`
	// LastSeen 最近一次发现时间
	LastSeen time.Time `json:"last_seen"`
	// OnlineSeconds 在线时长（秒），由 LastSeen - FirstSeen 计算
	OnlineSeconds int64 `json:"online_seconds"`
	// OS 设备操作系统（可选，由发现包带上）
	OS string `json:"os,omitempty"`
}

// IsOnline 判断设备是否在线
func (d Device) IsOnline() bool {
	return d.Status == DeviceStatusOnline
}

// CalcOnlineDuration 根据当前时间计算并填充 OnlineSeconds。
func (d *Device) CalcOnlineDuration(now time.Time) {
	if d.FirstSeen.IsZero() {
		d.OnlineSeconds = 0
		return
	}
	dur := now.Sub(d.FirstSeen)
	if dur < 0 {
		dur = 0
	}
	d.OnlineSeconds = int64(dur.Seconds())
}

// DeviceIDFromIPName 基于 IP 和名称生成稳定的设备 ID。
// 使用 FNV-1a，简单且足够区分局域网设备。
func DeviceIDFromIPName(ip, name string) string {
	const (
		offset32 = 2166136261
		prime32  = 16777619
	)
	s := ip + "|" + name
	var h uint32 = offset32
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime32
	}
	return "dev-" + itoaBase(uint64(h), 16)
}

// itoaBase 将非负整数按给定进制转成字符串，支持 2..16 进制。
func itoaBase(n uint64, base int) string {
	if base < 2 || base > 16 {
		base = 16
	}
	if n == 0 {
		return "0"
	}
	const digits = "0123456789abcdef"
	buf := make([]byte, 0, 16)
	for n > 0 {
		buf = append(buf, digits[n%uint64(base)])
		n /= uint64(base)
	}
	// 反转
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
