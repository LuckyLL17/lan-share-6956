package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lan-share/internal/model"
	"lan-share/internal/repository"
)

// ShareService 负责共享文件夹的业务逻辑：创建、查询、开关、列表与目录浏览。
// 它只处理本机共享，不涉及跨设备传输。
type ShareService struct {
	repo *repository.ShareRepository
}

// NewShareService 构造共享服务。
func NewShareService(repo *repository.ShareRepository) *ShareService {
	return &ShareService{repo: repo}
}

// CreateShareInput 创建共享的入参
type CreateShareInput struct {
	Alias      string                    `json:"alias"`
	Path       string                    `json:"path"`
	Permission model.SharePermission     `json:"permission"`
	Enabled    *bool                     `json:"enabled"`
}

// Create 创建一个共享。
// 会校验路径是否存在与是否为目录。
func (s *ShareService) Create(ctx context.Context, in CreateShareInput) (*model.Share, error) {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	sh := model.Share{
		Alias:      strings.TrimSpace(in.Alias),
		Path:       strings.TrimSpace(in.Path),
		Permission: in.Permission,
		Enabled:    enabled,
	}
	if sh.Permission == "" {
		sh.Permission = model.SharePermissionReadOnly
	}
	// 校验路径
	info, err := os.Stat(sh.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("path not exist: %s", sh.Path)
		}
		return nil, fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", sh.Path)
	}
	// 校验模型
	if err := sh.Validate(); err != nil {
		return nil, err
	}
	// 别名唯一性
	if existing, _ := s.repo.GetByAlias(ctx, sh.Alias); existing != nil {
		return nil, fmt.Errorf("alias %q already exists", sh.Alias)
	}
	if err := s.repo.Create(ctx, &sh); err != nil {
		return nil, err
	}
	return &sh, nil
}

// Update 更新共享字段。
func (s *ShareService) Update(ctx context.Context, id int64, in CreateShareInput) (*model.Share, error) {
	sh, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if sh == nil {
		return nil, model.ErrShareNotFound
	}
	if in.Alias != "" {
		sh.Alias = strings.TrimSpace(in.Alias)
	}
	if in.Path != "" {
		info, err := os.Stat(in.Path)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("invalid path: %s", in.Path)
		}
		sh.Path = in.Path
	}
	if in.Permission != "" {
		sh.Permission = in.Permission
	}
	if in.Enabled != nil {
		sh.Enabled = *in.Enabled
	}
	if err := sh.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, *sh); err != nil {
		return nil, err
	}
	return sh, nil
}

// Toggle 切换共享开关。
func (s *ShareService) Toggle(ctx context.Context, id int64, enabled bool) error {
	return s.repo.SetEnabled(ctx, id, enabled)
}

// Delete 删除共享（仅记录，不删除文件）。
func (s *ShareService) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Get 查询单个共享。
func (s *ShareService) Get(ctx context.Context, id int64) (*model.Share, error) {
	return s.repo.Get(ctx, id)
}

// GetByAlias 按别名查询共享。
func (s *ShareService) GetByAlias(ctx context.Context, alias string) (*model.Share, error) {
	return s.repo.GetByAlias(ctx, alias)
}

// List 列出共享。
func (s *ShareService) List(ctx context.Context, onlyEnabled bool) ([]model.Share, error) {
	return s.repo.List(ctx, onlyEnabled)
}

// ListItems 浏览共享目录内容（相对路径）。
// 返回的条目不暴露原始绝对路径。
// 路径与符号链接均会校验：若目录本身是符号链接且指向共享根之外，
// 或任何被列出的条目是逃逸符号链接，均会被拒绝。
func (s *ShareService) ListItems(ctx context.Context, alias, rel string) ([]model.ShareItem, error) {
	sh, err := s.repo.GetByAlias(ctx, alias)
	if err != nil {
		return nil, err
	}
	if sh == nil || !sh.Enabled {
		return nil, model.ErrShareNotFound
	}
	rel = cleanRelPath(rel)
	full, ok := sh.ResolvePath(rel)
	if !ok {
		return nil, errInvalidPath
	}
	// 校验目录真实路径仍在共享根内（防符号链接逃逸）
	realFull, ok := sh.SafePathOnDisk(full, false)
	if !ok {
		return nil, errInvalidPath
	}
	info, err := os.Stat(realFull)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", rel, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", rel)
	}
	entries, err := os.ReadDir(realFull)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	// 对每个被列出的条目校验符号链接：若指向共享根外，则跳过（不暴露未授权内容）。
	realRoot, err := filepath.EvalSymlinks(filepath.Clean(sh.Path))
	if err != nil {
		return nil, errInvalidPath
	}
	items := make([]model.ShareItem, 0, len(entries))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		entryPath := filepath.Join(realFull, e.Name())
		if li, lerr := os.Lstat(entryPath); lerr == nil && li.Mode()&os.ModeSymlink != 0 {
			realEntry, rerr := filepath.EvalSymlinks(entryPath)
			if rerr != nil || !isWithinRoot(realRoot, realEntry) {
				continue
			}
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, model.ShareItem{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
			RelPath: filepath.ToSlash(filepath.Join(rel, e.Name())),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

// isWithinRoot 报告 path 是否严格位于 root 内（含 root 自身）。
// 入参均需 filepath.Clean 过。
func isWithinRoot(root, path string) bool {
	if root == path {
		return true
	}
	if !strings.HasPrefix(path, root) {
		return false
	}
	return len(path) > len(root) && (path[len(root)] == os.PathSeparator)
}

// ResolveFile 解析共享内某文件的绝对路径，用于下载/预览。
// 同时返回共享对象以便权限判断。
// 该方法同时校验磁盘符号链接：若解析后的真实路径逃逸出共享根，
// 直接拒绝，避免通过符号链接访问未授权文件。
func (s *ShareService) ResolveFile(ctx context.Context, alias, rel string) (*model.Share, string, error) {
	sh, err := s.repo.GetByAlias(ctx, alias)
	if err != nil {
		return nil, "", err
	}
	if sh == nil || !sh.Enabled {
		return nil, "", model.ErrShareNotFound
	}
	rel = cleanRelPath(rel)
	full, ok := sh.ResolvePath(rel)
	if !ok {
		return nil, "", errInvalidPath
	}
	// 校验符号链接：真实路径必须仍在共享根内
	realFull, ok := sh.SafePathOnDisk(full, false)
	if !ok {
		return nil, "", errInvalidPath
	}
	return sh, realFull, nil
}

// ResolveUploadPath 解析上传目标绝对路径。
// 与 ResolveFile 不同：上传目标可能尚未存在，因此对父目录做符号链接校验，
// 再拼回文件名。最终路径必须仍在共享根内。
func (s *ShareService) ResolveUploadPath(sh *model.Share, rel string) (string, bool) {
	rel = cleanRelPath(rel)
	full, ok := sh.ResolvePath(rel)
	if !ok {
		return "", false
	}
	realFull, ok := sh.SafePathOnDisk(full, true)
	if !ok {
		return "", false
	}
	return realFull, true
}

// errInvalidPath 表示请求的路径越界或通过符号链接逃逸出共享根。
var errInvalidPath = errors.New("invalid path")

// StatFile 取得共享内某文件信息。
func (s *ShareService) StatFile(ctx context.Context, alias, rel string) (model.ShareItem, error) {
	_, full, err := s.ResolveFile(ctx, alias, rel)
	if err != nil {
		return model.ShareItem{}, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return model.ShareItem{}, err
	}
	return model.ShareItem{
		Name:    filepath.Base(full),
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
		RelPath: filepath.ToSlash(rel),
	}, nil
}

// cleanRelPath 清理相对路径，防止穿越。
func cleanRelPath(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "/")
	// 移除 .. 段
	parts := []string{}
	for _, p := range strings.Split(rel, "/") {
		if p == "" || p == "." || p == ".." {
			continue
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, "/")
}

// FormatTime 格式化时间用于展示
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
