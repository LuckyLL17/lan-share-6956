package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

// Config 全局配置结构，负责聚合本应用运行所需的所有可配置项。
// 通过单一实例 (Load/Get) 在整个程序生命周期内共享。
type Config struct {
	// Device 本机设备相关配置
	Device DeviceConfig
	// Server HTTP 服务相关配置
	Server ServerConfig
	// Network UDP 发现与文件传输相关配置
	Network NetworkConfig
	// Storage 数据库与文件存储路径相关配置
	Storage StorageConfig
}

// DeviceConfig 设备相关配置项
type DeviceConfig struct {
	// Name 本机设备名称，用于局域网内展示
	Name string
	// AutoReceive 是否自动接收其他设备推来的文件
	AutoReceive bool
}

// ServerConfig HTTP 服务相关配置项
type ServerConfig struct {
	// Host 监听主机地址
	Host string
	// Port HTTP 服务监听端口
	Port int
	// MaxUploadMB 单次上传文件大小上限（MB），0 表示不限
	MaxUploadMB int
}

// NetworkConfig 局域网通信配置项
type NetworkConfig struct {
	// UDPDiscoverPort UDP 设备发现广播端口
	UDPDiscoverPort int
	// FileServerPort 对外暴露文件服务的端口（与 HTTP Port 一致）
	FileServerPort int
	// DiscoverInterval 设备发现广播间隔（秒）
	DiscoverInterval int
	// DeviceTTL 设备离线判定时长（秒）
	DeviceTTL int
}

// StorageConfig 存储路径相关配置项
type StorageConfig struct {
	// DataDir 数据目录（存放 SQLite、日志等）
	DataDir string
	// ShareDir 默认共享根目录
	ShareDir string
	// DownloadDir 默认下载目录
	DownloadDir string
	// DBPath SQLite 数据库文件完整路径（由 DataDir 派生）
	DBPath string
}

var (
	cfgOnce sync.Once
	cfgIns  *Config
)

// defaults 返回各配置项的默认值。
// 默认值不依赖外部环境，确保在无配置文件时仍可启动。
func defaults() *Config {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	dataDir := filepath.Join(home, ".lan-share")
	return &Config{
		Device: DeviceConfig{
			Name:        defaultDeviceName(),
			AutoReceive: true,
		},
		Server: ServerConfig{
			Host:        "0.0.0.0",
			Port:        8765,
			MaxUploadMB: 4096,
		},
		Network: NetworkConfig{
			UDPDiscoverPort:  18765,
			FileServerPort:   8765,
			DiscoverInterval: 5,
			DeviceTTL:        15,
		},
		Storage: StorageConfig{
			DataDir:     dataDir,
			ShareDir:    filepath.Join(dataDir, "shares"),
			DownloadDir: filepath.Join(dataDir, "downloads"),
			DBPath:      filepath.Join(dataDir, "lanshare.db"),
		},
	}
}

// defaultDeviceName 返回默认设备名，优先使用主机名，失败时回退到 OS-平台名。
func defaultDeviceName() string {
	host, err := os.Hostname()
	if err == nil && host != "" {
		return host
	}
	return fmt.Sprintf("%s-device", runtime.GOOS)
}

// Load 读取并解析配置。
// 优先级：环境变量 > 同目录下 config.yaml > 默认值。
// 同时确保所有存储目录存在。该方法线程安全，只会执行一次实际加载。
func Load() (*Config, error) {
	var err error
	cfgOnce.Do(func() {
		cfgIns = defaults()
		err = loadFromViper(cfgIns)
		if err != nil {
			return
		}
		err = ensureDirs(cfgIns)
	})
	return cfgIns, err
}

// loadFromViper 使用 viper 读取环境变量与可选的 config.yaml，覆盖默认值。
func loadFromViper(c *Config) error {
	v := viper.New()
	v.SetEnvPrefix("LANSHARE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath(c.Storage.DataDir)
	if e := v.ReadInConfig(); e != nil {
		// 配置文件可选，找不到时不视为错误
		if _, ok := e.(viper.ConfigFileNotFoundError); !ok {
			// 忽略其他读取错误，使用默认值即可
		}
	}

	if v.IsSet("device.name") {
		c.Device.Name = v.GetString("device.name")
	}
	if v.IsSet("device.auto_receive") {
		c.Device.AutoReceive = v.GetBool("device.auto_receive")
	}
	if v.IsSet("server.host") {
		c.Server.Host = v.GetString("server.host")
	}
	if v.IsSet("server.port") {
		c.Server.Port = v.GetInt("server.port")
	}
	if v.IsSet("server.max_upload_mb") {
		c.Server.MaxUploadMB = v.GetInt("server.max_upload_mb")
	}
	if v.IsSet("network.udp_discover_port") {
		c.Network.UDPDiscoverPort = v.GetInt("network.udp_discover_port")
	}
	if v.IsSet("network.discover_interval") {
		c.Network.DiscoverInterval = v.GetInt("network.discover_interval")
	}
	if v.IsSet("network.device_ttl") {
		c.Network.DeviceTTL = v.GetInt("network.device_ttl")
	}
	if v.IsSet("storage.data_dir") {
		c.Storage.DataDir = v.GetString("storage.data_dir")
		c.Storage.DBPath = filepath.Join(c.Storage.DataDir, "lanshare.db")
	}
	if v.IsSet("storage.share_dir") {
		c.Storage.ShareDir = v.GetString("storage.share_dir")
	}
	if v.IsSet("storage.download_dir") {
		c.Storage.DownloadDir = v.GetString("storage.download_dir")
	}
	return nil
}

// ensureDirs 创建所有需要的存储目录。
func ensureDirs(c *Config) error {
	for _, dir := range []string{c.Storage.DataDir, c.Storage.ShareDir, c.Storage.DownloadDir} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	return nil
}

// Get 返回已加载的全局配置实例。
// 如果尚未调用 Load，会触发一次加载。
func Get() *Config {
	if cfgIns == nil {
		_, _ = Load()
	}
	return cfgIns
}

// SetDeviceName 修改运行期设备名，仅作用于内存，不持久化。
func SetDeviceName(name string) {
	if cfgIns != nil && name != "" {
		cfgIns.Device.Name = name
	}
}

// SetAutoReceive 修改运行期自动接收开关。
func SetAutoReceive(on bool) {
	if cfgIns != nil {
		cfgIns.Device.AutoReceive = on
	}
}
