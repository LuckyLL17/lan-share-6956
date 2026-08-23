package model

import "time"

// TransferDirection 传输方向
type TransferDirection string

const (
	// TransferDirectionDownload 从其他设备下载
	TransferDirectionDownload TransferDirection = "download"
	// TransferDirectionUpload 上传到其他设备（对方需有写权限）
	TransferDirectionUpload TransferDirection = "upload"
)

// TransferStatus 传输状态
type TransferStatus string

const (
	TransferStatusQueued   TransferStatus = "queued"   // 排队中
	TransferStatusRunning  TransferStatus = "running"  // 传输中
	TransferStatusPaused   TransferStatus = "paused"   // 暂停
	TransferStatusDone     TransferStatus = "done"     // 完成
	TransferStatusFailed   TransferStatus = "failed"   // 失败
	TransferStatusCanceled TransferStatus = "canceled" // 已取消
)

// Transfer 一次文件传输任务的记录。
// 支持断点续传：通过 BytesTransferred 与目标文件大小对比继续传输。
type Transfer struct {
	ID int64 `json:"id"`
	// Direction 方向
	Direction TransferDirection `json:"direction"`
	// PeerDeviceID 对端设备 ID
	PeerDeviceID string `json:"peer_device_id"`
	// PeerIP 对端 IP
	PeerIP string `json:"peer_ip"`
	// PeerPort 对端文件服务端口
	PeerPort int `json:"peer_port"`
	// ShareAlias 对端共享别名（下载场景）
	ShareAlias string `json:"share_alias,omitempty"`
	// RemotePath 远端相对路径
	RemotePath string `json:"remote_path"`
	// LocalPath 本地保存/上传路径
	LocalPath string `json:"local_path"`
	// FileName 文件名
	FileName string `json:"file_name"`
	// FileSize 文件总字节数
	FileSize int64 `json:"file_size"`
	// BytesTransferred 已传输字节数
	BytesTransferred int64 `json:"bytes_transferred"`
	// Status 当前状态
	Status TransferStatus `json:"status"`
	// ErrMsg 失败原因
	ErrMsg string `json:"err_msg,omitempty"`
	// CreatedAt 创建时间
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 更新时间
	UpdatedAt time.Time `json:"updated_at"`
	// FinishedAt 完成时间
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

// Progress 返回进度百分比（0-100）
func (t Transfer) Progress() float64 {
	if t.FileSize <= 0 {
		return 0
	}
	p := float64(t.BytesTransferred) / float64(t.FileSize) * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// IsTerminal 判断是否处于终态
func (t Transfer) IsTerminal() bool {
	switch t.Status {
	case TransferStatusDone, TransferStatusFailed, TransferStatusCanceled:
		return true
	}
	return false
}

// IsResumable 是否可断点续传
func (t Transfer) IsResumable() bool {
	if t.FileSize <= 0 {
		return false
	}
	if t.BytesTransferred >= t.FileSize {
		return false
	}
	switch t.Status {
	case TransferStatusPaused, TransferStatusFailed, TransferStatusQueued:
		return true
	}
	return false
}

// CanCancel 是否可取消
func (t Transfer) CanCancel() bool {
	return !t.IsTerminal()
}

// CanPause 是否可暂停
func (t Transfer) CanPause() bool {
	return t.Status == TransferStatusRunning || t.Status == TransferStatusQueued
}

// IsPausedState reports whether the persisted transfer state represents a pause.
func (t Transfer) IsPausedState() bool {
	return t.Status == TransferStatusPaused
}

// Validate 创建传输时的基本校验
func (t Transfer) Validate() error {
	if t.PeerIP == "" {
		return ErrTransferPeerEmpty
	}
	if t.FileName == "" {
		return ErrTransferNameEmpty
	}
	if t.Direction == "" || (t.Direction != TransferDirectionDownload && t.Direction != TransferDirectionUpload) {
		return ErrTransferDirectionInvalid
	}
	return nil
}

// TransferError 传输相关错误
type TransferError struct {
	Code string
	Msg  string
}

func (e *TransferError) Error() string { return e.Msg }

var (
	ErrTransferPeerEmpty        = &TransferError{Code: "PEER_EMPTY", Msg: "peer ip is empty"}
	ErrTransferNameEmpty        = &TransferError{Code: "NAME_EMPTY", Msg: "file name is empty"}
	ErrTransferDirectionInvalid = &TransferError{Code: "DIR_INVALID", Msg: "direction must be download or upload"}
	ErrTransferNotFound         = &TransferError{Code: "NOT_FOUND", Msg: "transfer not found"}
	ErrTransferNotResumable     = &TransferError{Code: "NOT_RESUMABLE", Msg: "transfer is not resumable"}
)
