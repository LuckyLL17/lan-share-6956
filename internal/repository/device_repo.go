// Package repository 实现对持久化层的访问。
// 每个 repo 文件只负责一种实体，符合单一职责原则。
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"lan-share/internal/model"
)

// DeviceRepository 负责设备记录的增删改查。
type DeviceRepository struct {
	db *sql.DB
}

// NewDeviceRepository 构造设备仓库
func NewDeviceRepository(db *sql.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// Upsert 插入或更新设备记录（按 ID 唯一）。
// 首次发现会写入 first_seen，后续仅更新 last_seen 与状态。
func (r *DeviceRepository) Upsert(ctx context.Context, d model.Device) error {
	if d.ID == "" {
		return errors.New("device id empty")
	}
	now := time.Now().UTC()
	if d.FirstSeen.IsZero() {
		d.FirstSeen = now
	}
	d.LastSeen = now
	if d.Status == "" {
		d.Status = model.DeviceStatusOnline
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO devices (id, name, ip, port, status, first_seen, last_seen, os)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			ip=excluded.ip,
			port=excluded.port,
			status=excluded.status,
			last_seen=excluded.last_seen,
			os=excluded.os;
	`, d.ID, d.Name, d.IP, d.Port, string(d.Status), d.FirstSeen, d.LastSeen, d.OS)
	return err
}

// SetStatus 仅更新设备状态。
func (r *DeviceRepository) SetStatus(ctx context.Context, id string, status model.DeviceStatus) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE devices SET status=?, last_seen=? WHERE id=?`,
		string(status), time.Now().UTC(), id)
	return err
}

// Get 按 ID 查询设备。
func (r *DeviceRepository) Get(ctx context.Context, id string) (*model.Device, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, name, ip, port, status, first_seen, last_seen, os FROM devices WHERE id=?`, id)
	var d model.Device
	var status string
	if err := row.Scan(&d.ID, &d.Name, &d.IP, &d.Port, &status, &d.FirstSeen, &d.LastSeen, &d.OS); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	d.Status = model.DeviceStatus(status)
	d.CalcOnlineDuration(time.Now())
	return &d, nil
}

// ListByStatus 按状态列出设备，按 last_seen 降序。
func (r *DeviceRepository) ListByStatus(ctx context.Context, status model.DeviceStatus) ([]model.Device, error) {
	var (
		q    string
		args []interface{}
	)
	if status == "" {
		q = `SELECT id, name, ip, port, status, first_seen, last_seen, os FROM devices ORDER BY last_seen DESC`
	} else {
		q = `SELECT id, name, ip, port, status, first_seen, last_seen, os FROM devices WHERE status=? ORDER BY last_seen DESC`
		args = []interface{}{string(status)}
	}
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Device, 0, 32)
	for rows.Next() {
		var d model.Device
		var s string
		if err := rows.Scan(&d.ID, &d.Name, &d.IP, &d.Port, &s, &d.FirstSeen, &d.LastSeen, &d.OS); err != nil {
			return nil, err
		}
		d.Status = model.DeviceStatus(s)
		d.CalcOnlineDuration(time.Now())
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListAll 列出所有设备。
func (r *DeviceRepository) ListAll(ctx context.Context) ([]model.Device, error) {
	return r.ListByStatus(ctx, "")
}

// Delete 删除设备。
func (r *DeviceRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM devices WHERE id=?`, id)
	return err
}

// MarkStale 将超过 ttl 没有心跳的设备标记为离线。
func (r *DeviceRepository) MarkStale(ctx context.Context, ttl time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-ttl)
	res, err := r.db.ExecContext(ctx,
		`UPDATE devices SET status='offline' WHERE status='online' AND last_seen < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Count 返回设备总数。
func (r *DeviceRepository) Count(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices`).Scan(&n)
	return n, err
}
