package model

import (
	"path/filepath"
	"strings"
	"time"
)

// SharePermission 共享权限类型
type SharePermission string

const (
	// SharePermissionReadOnly 只读：可浏览、下载
	SharePermissionReadOnly SharePermission = "readonly"
	// SharePermissionReadWrite 可写：可上传、删除、下载
	SharePermissionReadWrite SharePermission = "readwrite"
)

// Share 描述一个本机对外共享的文件夹。
// 通过该结构，其他设备可知道有哪些目录可访问及其权限。
type Share struct {
	// ID 主键
	ID int64 `json:"id"`
	// Alias 共享别名，便于在其他设备展示
	Alias string `json:"alias"`
	// Path 本机真实路径（不对外暴露原始路径）
	Path string `json:"-"`
	// Permission 权限模式
	Permission SharePermission `json:"permission"`
	// Enabled 是否开启共享，关闭后其他设备不可见
	Enabled bool `json:"enabled"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
}

// IsReadable 是否允许读取
func (s Share) IsReadable() bool {
	return s.Enabled
}

// IsWritable 是否允许写入
func (s Share) IsWritable() bool {
	return s.Enabled && s.Permission == SharePermissionReadWrite
}

// ResolvePath turns a share-relative path into a filesystem path.
func (s Share) ResolvePath(rel string) (string, bool) {
	root := filepath.Clean(s.Path)
	full := filepath.Clean(filepath.Join(root, rel))
	return full, strings.HasPrefix(full, root)
}

// Validate 创建/更新共享时的基本校验
func (s Share) Validate() error {
	switch s.Permission {
	case SharePermissionReadOnly, SharePermissionReadWrite:
	default:
		return ErrSharePermissionInvalid
	}
	if s.Alias == "" {
		return ErrShareAliasEmpty
	}
	if s.Path == "" {
		return ErrSharePathEmpty
	}
	return nil
}

// ShareError 共享相关错误类型
type ShareError struct {
	Code string
	Msg  string
}

func (e *ShareError) Error() string {
	return e.Msg
}

// 预定义错误
var (
	ErrShareAliasEmpty        = &ShareError{Code: "ALIAS_EMPTY", Msg: "share alias is empty"}
	ErrSharePathEmpty         = &ShareError{Code: "PATH_EMPTY", Msg: "share path is empty"}
	ErrSharePermissionInvalid = &ShareError{Code: "PERMISSION_INVALID", Msg: "permission must be readonly or readwrite"}
	ErrShareNotFound          = &ShareError{Code: "NOT_FOUND", Msg: "share not found"}
)

// ShareItem 共享文件夹内的一个条目（文件或目录），用于目录浏览。
type ShareItem struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	// Path 相对共享根目录的相对路径（用正斜杠）
	RelPath string `json:"rel_path"`
}
