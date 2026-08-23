package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lan-share/config"
	"lan-share/internal/model"
	"lan-share/internal/network"
	"lan-share/internal/repository"
)

// TransferService 负责文件传输的调度与执行。
// 支持断点续传、并发任务管理、暂停/取消。
type TransferService struct {
	cfg     *config.Config
	repo    *repository.TransferRepository
	client  *network.HTTPFileClient
	httpCli *http.Client

	mu     sync.Mutex
	tasks  map[int64]*transferTask // id -> 运行态任务
	cancel map[int64]context.CancelFunc
	pause  map[int64]chan struct{}
}

// transferTask 内部运行态
type transferTask struct {
	transferID int64
	dir        model.TransferDirection
	cancel     context.CancelFunc
	paused     bool
}

// NewTransferService 构造传输服务。
func NewTransferService(cfg *config.Config, repo *repository.TransferRepository) *TransferService {
	return &TransferService{
		cfg:     cfg,
		repo:    repo,
		client:  network.NewHTTPFileClient(cfg),
		httpCli: &http.Client{Timeout: 0},
		tasks:   make(map[int64]*transferTask),
		cancel:  make(map[int64]context.CancelFunc),
		pause:   make(map[int64]chan struct{}),
	}
}

// CreateTransferInput 创建传输的入参
type CreateTransferInput struct {
	Direction   model.TransferDirection `json:"direction"`
	PeerDeviceID string                 `json:"peer_device_id"`
	PeerIP       string                 `json:"peer_ip"`
	PeerPort     int                    `json:"peer_port"`
	ShareAlias   string                 `json:"share_alias"`
	RemotePath   string                 `json:"remote_path"`
	LocalPath    string                 `json:"local_path"`
	FileName     string                 `json:"file_name"`
	FileSize     int64                  `json:"file_size"`
	StartNow     bool                   `json:"start_now"`
}

// Create 创建并（可选）立即启动传输任务。
func (s *TransferService) Create(ctx context.Context, in CreateTransferInput) (*model.Transfer, error) {
	if in.PeerPort == 0 {
		in.PeerPort = s.cfg.Network.FileServerPort
	}
	if in.FileName == "" && in.RemotePath != "" {
		in.FileName = filepath.Base(in.RemotePath)
	}
	if in.LocalPath == "" {
		in.LocalPath = filepath.Join(s.cfg.Storage.DownloadDir, in.FileName)
	}
	t := model.Transfer{
		Direction:       in.Direction,
		PeerDeviceID:    in.PeerDeviceID,
		PeerIP:          in.PeerIP,
		PeerPort:         in.PeerPort,
		ShareAlias:      in.ShareAlias,
		RemotePath:      in.RemotePath,
		LocalPath:       in.LocalPath,
		FileName:        in.FileName,
		FileSize:        in.FileSize,
		Status:          model.TransferStatusQueued,
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, &t); err != nil {
		return nil, err
	}
	if in.StartNow {
		go s.run(context.Background(), t)
	}
	return &t, nil
}

// Start 启动一个已存在的传输任务（支持断点续传）。
func (s *TransferService) Start(ctx context.Context, id int64) error {
	t, err := s.repo.Get(ctx, id)
	if err != nil || t == nil {
		return model.ErrTransferNotFound
	}
	if !t.IsResumable() && t.Status != model.TransferStatusQueued && t.Status != model.TransferStatusPaused {
		return model.ErrTransferNotResumable
	}
	go s.run(context.Background(), *t)
	return nil
}

// Pause 暂停任务。
func (s *TransferService) Pause(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cf, ok := s.cancel[id]; ok {
		cf()
	}
	_ = s.repo.SetStatus(ctx, id, model.TransferStatusPaused, "")
	return nil
}

// Cancel 取消任务。
func (s *TransferService) Cancel(ctx context.Context, id int64) error {
	s.mu.Lock()
	if cf, ok := s.cancel[id]; ok {
		cf()
	}
	delete(s.tasks, id)
	delete(s.cancel, id)
	delete(s.pause, id)
	s.mu.Unlock()
	return s.repo.SetStatus(ctx, id, model.TransferStatusCanceled, "canceled by user")
}

// Get 查询传输记录。
func (s *TransferService) Get(ctx context.Context, id int64) (*model.Transfer, error) {
	return s.repo.Get(ctx, id)
}

// List 列出传输记录。
func (s *TransferService) List(ctx context.Context, limit int) ([]model.Transfer, error) {
	return s.repo.List(ctx, limit)
}

// ListByStatus 按状态列出。
func (s *TransferService) ListByStatus(ctx context.Context, status model.TransferStatus) ([]model.Transfer, error) {
	return s.repo.ListByStatus(ctx, status)
}

// Delete 删除传输记录（终态才允许）。
func (s *TransferService) Delete(ctx context.Context, id int64) error {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if t == nil {
		return model.ErrTransferNotFound
	}
	if !t.IsTerminal() {
		return fmt.Errorf("transfer not terminal: %s", t.Status)
	}
	return s.repo.Delete(ctx, id)
}

// run 执行实际传输，调用 HTTP 文件客户端。
func (s *TransferService) run(ctx context.Context, t model.Transfer) {
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.mu.Lock()
	s.tasks[t.ID] = &transferTask{transferID: t.ID, dir: t.Direction, cancel: cancel}
	s.cancel[t.ID] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.tasks, t.ID)
		delete(s.cancel, t.ID)
		s.mu.Unlock()
	}()

	_ = s.repo.SetStatus(taskCtx, t.ID, model.TransferStatusRunning, "")

	var err error
	switch t.Direction {
	case model.TransferDirectionDownload:
		err = s.doDownload(taskCtx, t)
	case model.TransferDirectionUpload:
		err = s.doUpload(taskCtx, t)
	}

	if err != nil {
		if errors.Is(err, context.Canceled) {
			current, getErr := s.repo.Get(context.Background(), t.ID)
			if getErr == nil && current != nil && (current.Status == model.TransferStatusPaused || current.Status == model.TransferStatusCanceled) {
				return
			}
		}
		_ = s.repo.SetStatus(context.Background(), t.ID, model.TransferStatusFailed, err.Error())
		log.Printf("[transfer] #%d failed: %v", t.ID, err)
		return
	}
	_ = s.repo.SetStatus(context.Background(), t.ID, model.TransferStatusDone, "")
	log.Printf("[transfer] #%d done", t.ID)
}

// doDownload 执行下载：支持 Range 断点续传。
func (s *TransferService) doDownload(ctx context.Context, t model.Transfer) error {
	// 打开本地文件（追加模式）
	flag := os.O_CREATE | os.O_WRONLY
	start := t.BytesTransferred
	if start > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(t.LocalPath, flag, 0o644)
	if err != nil {
		return fmt.Errorf("open local: %w", err)
	}
	defer f.Close()

	resp, err := s.client.Download(ctx, t.PeerIP, t.PeerPort, t.ShareAlias, t.RemotePath, start)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote status %d", resp.StatusCode)
	}

	// 若远端返回 200 而非 206，说明不支持续传，重置本地文件
	if resp.StatusCode == http.StatusOK && start > 0 {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if err := f.Truncate(0); err != nil {
			return err
		}
		start = 0
	}

	// 若未指定大小，则尝试从 Content-Length 取
	total := t.FileSize
	if total <= 0 && resp.ContentLength > 0 {
		total = resp.ContentLength + start
		_ = s.repo.SetProgress(ctx, t.ID, start)
	}

	buf := make([]byte, 32*1024)
	written := start
	lastUpdate := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if time.Since(lastUpdate) > 500*time.Millisecond {
				_ = s.repo.SetProgress(ctx, t.ID, written)
				lastUpdate = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	_ = s.repo.SetProgress(ctx, t.ID, written)
	// 总大小修正
	if total > 0 {
		_ = s.repo.SetProgress(ctx, t.ID, written)
	}
	return nil
}

// doUpload 执行上传：通过 PUT 到对端的 /api/v1/uploads。
func (s *TransferService) doUpload(ctx context.Context, t model.Transfer) error {
	f, err := os.Open(t.LocalPath)
	if err != nil {
		return fmt.Errorf("open local: %w", err)
	}
	defer f.Close()
	start := t.BytesTransferred
	if start > 0 {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return err
		}
	}
	written, err := s.client.Upload(ctx, t.PeerIP, t.PeerPort, t.ShareAlias, t.RemotePath, f, start,
		func(n int64) {
			_ = s.repo.SetProgress(ctx, t.ID, n+start)
		})
	if err != nil {
		return err
	}
	_ = s.repo.SetProgress(ctx, t.ID, written+start)
	return nil
}

// Progress 返回当前进度信息。
func (s *TransferService) Progress(ctx context.Context, id int64) (model.Transfer, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil || t == nil {
		return model.Transfer{}, model.ErrTransferNotFound
	}
	return *t, nil
}

// ClearHistory 清理所有终态记录。
func (s *TransferService) ClearHistory(ctx context.Context) error {
	for _, st := range []model.TransferStatus{
		model.TransferStatusDone, model.TransferStatusFailed, model.TransferStatusCanceled,
	} {
		list, err := s.repo.ListByStatus(ctx, st)
		if err != nil {
			return err
		}
		for _, t := range list {
			if err := s.repo.Delete(ctx, t.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
