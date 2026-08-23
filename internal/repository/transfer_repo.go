package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"lan-share/internal/model"
)

// TransferRepository 负责传输任务的持久化访问。
type TransferRepository struct {
	db *sql.DB
}

// NewTransferRepository 构造传输仓库
func NewTransferRepository(db *sql.DB) *TransferRepository {
	return &TransferRepository{db: db}
}

// Create 创建传输记录，回写 ID。
func (r *TransferRepository) Create(ctx context.Context, t *model.Transfer) error {
	if t == nil {
		return errors.New("nil transfer")
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.Status == "" {
		t.Status = model.TransferStatusQueued
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO transfers (
			direction, peer_device_id, peer_ip, peer_port, share_alias,
			remote_path, local_path, file_name, file_size, bytes_transferred,
			status, err_msg, created_at, updated_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`,
		string(t.Direction), t.PeerDeviceID, t.PeerIP, t.PeerPort, t.ShareAlias,
		t.RemotePath, t.LocalPath, t.FileName, t.FileSize, t.BytesTransferred,
		string(t.Status), t.ErrMsg, t.CreatedAt, t.UpdatedAt, nullableTime(t.FinishedAt),
	)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	t.ID = id
	return nil
}

// Update 更新进度与状态。
func (r *TransferRepository) Update(ctx context.Context, t model.Transfer) error {
	t.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE transfers SET
			bytes_transferred=?, status=?, err_msg=?, updated_at=?, finished_at=?
		WHERE id=?;
	`, t.BytesTransferred, string(t.Status), t.ErrMsg, t.UpdatedAt, nullableTime(t.FinishedAt), t.ID)
	return err
}

// SetStatus 仅更新状态。
// 暂停为非终态：不设置 finished_at，保留断点续传能力。
func (r *TransferRepository) SetStatus(ctx context.Context, id int64, status model.TransferStatus, errMsg string) error {
	finishedAt := (*time.Time)(nil)
	if status == model.TransferStatusDone || status == model.TransferStatusFailed || status == model.TransferStatusCanceled {
		now := time.Now().UTC()
		finishedAt = &now
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE transfers SET status=?, err_msg=?, updated_at=?, finished_at=? WHERE id=?`,
		string(status), errMsg, time.Now().UTC(), nullableTimePtr(finishedAt), id)
	return err
}

// SetProgress 更新已传输字节。
func (r *TransferRepository) SetProgress(ctx context.Context, id int64, bytes int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE transfers SET bytes_transferred=?, updated_at=? WHERE id=?`,
		bytes, time.Now().UTC(), id)
	return err
}

// Delete 删除传输记录。
func (r *TransferRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM transfers WHERE id=?`, id)
	return err
}

// Get 按 ID 查询传输。
func (r *TransferRepository) Get(ctx context.Context, id int64) (*model.Transfer, error) {
	row := r.db.QueryRowContext(ctx, transferSelect+` WHERE id=?`, id)
	return scanTransfer(row)
}

// List 列出传输记录，按创建时间倒序，限制条数。
func (r *TransferRepository) List(ctx context.Context, limit int) ([]model.Transfer, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, transferSelect+` O`+`RDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransfers(rows)
}

// ListByStatus 按状态列出传输。
func (r *TransferRepository) ListByStatus(ctx context.Context, status model.TransferStatus) ([]model.Transfer, error) {
	rows, err := r.db.QueryContext(ctx, transferSelect+` WHERE status=? O`+`RDER BY created_at DESC`, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTransfers(rows)
}

const transferSelect = `SELECT id, direction, peer_device_id, peer_ip, peer_port, share_alias,
	remote_path, local_path, file_name, file_size, bytes_transferred,
	status, err_msg, created_at, updated_at, finished_at FROM transfers`

func scanTransfer(s scanner) (*model.Transfer, error) {
	var t model.Transfer
	var dir, status string
	var finished sql.NullTime
	if err := s.Scan(&t.ID, &dir, &t.PeerDeviceID, &t.PeerIP, &t.PeerPort, &t.ShareAlias,
		&t.RemotePath, &t.LocalPath, &t.FileName, &t.FileSize, &t.BytesTransferred,
		&status, &t.ErrMsg, &t.CreatedAt, &t.UpdatedAt, &finished); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.Direction = model.TransferDirection(dir)
	t.Status = model.TransferStatus(status)
	if finished.Valid {
		t.FinishedAt = finished.Time
	}
	return &t, nil
}

func scanTransfers(rows *sql.Rows) ([]model.Transfer, error) {
	out := make([]model.Transfer, 0, 16)
	for rows.Next() {
		var t model.Transfer
		var dir, status string
		var finished sql.NullTime
		if err := rows.Scan(&t.ID, &dir, &t.PeerDeviceID, &t.PeerIP, &t.PeerPort, &t.ShareAlias,
			&t.RemotePath, &t.LocalPath, &t.FileName, &t.FileSize, &t.BytesTransferred,
			&status, &t.ErrMsg, &t.CreatedAt, &t.UpdatedAt, &finished); err != nil {
			return nil, err
		}
		t.Direction = model.TransferDirection(dir)
		t.Status = model.TransferStatus(status)
		if finished.Valid {
			t.FinishedAt = finished.Time
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// nullableTime 将零时间映射为 NULL。
func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// nullableTimePtr 处理可空时间指针
func nullableTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return *t
}
