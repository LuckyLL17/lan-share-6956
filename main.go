// Package main 是 lan-share 应用的入口。
// 负责加载配置、初始化各层依赖、启动 HTTP 服务与 UDP 发现。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"lan-share/api"
	"lan-share/config"
	"lan-share/internal/db"
	"lan-share/internal/handler"
	"lan-share/internal/network"
	"lan-share/internal/repository"
	"lan-share/internal/service"
)

// buildInfo 编译期可注入的构建信息
var (
	version   = "1.0.0"
	buildTime = "unknown"
)

func main() {
	cfgPath := flag.String("config", "", "path to config.yaml")
	port := flag.Int("port", 0, "override http port")
	showVer := flag.Bool("v", false, "show version")
	flag.Parse()

	if *showVer {
		fmt.Printf("lan-share %s (build %s)\n", version, buildTime)
		return
	}

	// 1. 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[main] load config: %v", err)
	}
	if *cfgPath != "" {
		log.Printf("[main] config file at %s is optional; using defaults + env", *cfgPath)
	}
	if *port > 0 {
		cfg.Server.Port = *port
		cfg.Network.FileServerPort = *port
	}
	log.Printf("[main] config loaded, device=%s port=%d", cfg.Device.Name, cfg.Server.Port)

	// 2. 初始化数据库
	database, err := db.Open(cfg.Storage.DBPath)
	if err != nil {
		log.Fatalf("[main] open db: %v", err)
	}
	defer db.Close()

	// 3. 构造 repository
	devRepo := repository.NewDeviceRepository(database)
	shareRepo := repository.NewShareRepository(database)
	transferRepo := repository.NewTransferRepository(database)
	msgRepo := repository.NewMessageRepository(database)

	// 4. 构造 network
	udp := network.NewUDPDiscovery(cfg.Network.UDPDiscoverPort, cfg.Device.Name)

	// 5. 构造 service
	discoverSvc := service.NewDiscoverService(cfg, udp, devRepo, msgRepo)
	shareSvc := service.NewShareService(shareRepo)
	transferSvc := service.NewTransferService(cfg, transferRepo)
	previewSvc := service.NewPreviewService()

	// 6. 构造 handler
	devH := handler.NewDeviceHandler(discoverSvc)
	shareH := handler.NewShareHandler(shareSvc)
	transferH := handler.NewTransferHandler(transferSvc)
	fileH := handler.NewFileHandler(shareSvc, previewSvc)
	msgH := handler.NewMessageHandler(msgRepo, discoverSvc)
	settingsH := handler.NewSettingsHandler(cfg)

	// 7. 构造路由
	webDir := resolveWebDir()
	r := api.NewRouter(devH, shareH, transferH, fileH, msgH, settingsH, webDir)
	engine := r.Setup()

	// 8. 启动后台服务
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := discoverSvc.Start(rootCtx); err != nil {
		log.Fatalf("[main] start discover: %v", err)
	}
	defer discoverSvc.Stop()

	// 9. 启动 HTTP
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("[main] HTTP listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[main] http: %v", err)
		}
	}()

	// 10. 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[main] received signal %s, shutting down...", sig)

	shutdownCtx, sdCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sdCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[main] shutdown: %v", err)
	}
	cancel()
	log.Println("[main] bye")
}

// resolveWebDir 解析静态前端目录。
// 优先使用可执行文件同级 web 目录，其次使用当前工作目录 web。
func resolveWebDir() string {
	exe, err := os.Executable()
	if err == nil {
		p := filepath.Join(filepath.Dir(exe), "web")
		if isDir(p) {
			return p
		}
	}
	if isDir("web") {
		abs, _ := filepath.Abs("web")
		return abs
	}
	return ""
}

// isDir 判断路径是否为目录
func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
