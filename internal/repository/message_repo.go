package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"lan-share/internal/model"
)

// MessageRepository 负责设备消息的持久化访问。
type MessageRepository struct {
	db *sql.DB
}

// NewMessageRepository 构造消息仓库
func NewMessageRepository(db *sql.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// Create 写入一条收到的消息。
func (r *MessageRepository) Create(ctx context.Context, m *model.Message) error {
	if m == nil {
		return errors.New("nil message")
	}
	m.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO messages (from_ip, from_name, to_ip, content, read, created_at)
		VALUES (?, ?, ?, ?, ?, ?);
	`, m.FromIP, m.FromName, m.ToIP, m.Content, boolToInt(m.Read), m.CreatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	m.ID = id
	return nil
}

// List 列出消息，按时间倒序，限制条数。
func (r *MessageRepository) List(ctx context.Context, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, from_ip, from_name, to_ip, content, read, created_at
		 FROM messages O`+`RDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Message, 0, 16)
	for rows.Next() {
		var m model.Message
		var read int
		if err := rows.Scan(&m.ID, &m.FromIP, &m.FromName, &m.ToIP, &m.Content, &read, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Read = read == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

// MarkRead 标记消息为已读。
func (r *MessageRepository) MarkRead(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE messages SET read=1 WHERE id=?`, id)
	return err
}

// MarkAllRead 标记所有消息为已读。
func (r *MessageRepository) MarkAllRead(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE messages SET read=1 WHERE read=0`)
	return err
}

// CountUnread 统计未读消息数。
func (r *MessageRepository) CountUnread(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE read=0`).Scan(&n)
	return n, err
}

// Delete 删除消息。
func (r *MessageRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM messages WHERE id=?`, id)
	return err
}
