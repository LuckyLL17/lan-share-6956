package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"lan-share/internal/model"
)

// ShareRepository 负责共享文件夹的持久化访问。
type ShareRepository struct {
	db *sql.DB
}

// NewShareRepository 构造共享仓库
func NewShareRepository(db *sql.DB) *ShareRepository {
	return &ShareRepository{db: db}
}

// Create 创建一条共享记录，回写 ID 与时间。
func (r *ShareRepository) Create(ctx context.Context, s *model.Share) error {
	if s == nil {
		return errors.New("nil share")
	}
	now := time.Now().UTC()
	s.CreatedAt = now
	s.UpdatedAt = now
	if s.Permission == "" {
		s.Permission = model.SharePermissionReadOnly
	}
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO shares (alias, path, permission, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?);
	`, s.Alias, s.Path, string(s.Permission), boolToInt(s.Enabled), s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	s.ID = id
	return nil
}

// Update 更新共享（除 ID 外字段）。
func (r *ShareRepository) Update(ctx context.Context, s model.Share) error {
	s.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, `
		UPDATE shares SET
			alias=?, path=?, permission=?, enabled=?, updated_at=?
		WHERE id=?;
	`, s.Alias, s.Path, string(s.Permission), boolToInt(s.Enabled), s.UpdatedAt, s.ID)
	return err
}

// SetEnabled 仅开关共享状态。
func (r *ShareRepository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE shares SET enabled=?, updated_at=? WHERE id=?`,
		boolToInt(enabled), time.Now().UTC(), id)
	return err
}

// Delete 删除共享。
func (r *ShareRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM shares WHERE id=?`, id)
	return err
}

// Get 按 ID 查询共享。
func (r *ShareRepository) Get(ctx context.Context, id int64) (*model.Share, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, alias, path, permission, enabled, created_at, updated_at
		FROM shares WHERE id=?`, id)
	return scanShare(row)
}

// GetByAlias 按别名查询共享。
func (r *ShareRepository) GetByAlias(ctx context.Context, alias string) (*model.Share, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, alias, path, permission, enabled, created_at, updated_at
		FROM shares WHERE alias=?`, alias)
	return scanShare(row)
}

// List 列出所有共享，可选仅返回开启的。
func (r *ShareRepository) List(ctx context.Context, onlyEnabled bool) ([]model.Share, error) {
	q := `SELECT id, alias, path, permission, enabled, created_at, updated_at FROM shares`
	args := []interface{}{}
	if onlyEnabled {
		q += ` WHERE enabled=1`
	}
	q += ` O` + `RDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Share, 0, 8)
	for rows.Next() {
		var s model.Share
		var perm string
		var en int
		if err := rows.Scan(&s.ID, &s.Alias, &s.Path, &perm, &en, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Permission = model.SharePermission(perm)
		s.Enabled = en == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

// scanner 抽象 sql.Row / sql.Rows 的 Scan 接口。
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanShare 通用扫描共享记录
func scanShare(s scanner) (*model.Share, error) {
	var sh model.Share
	var perm string
	var en int
	if err := s.Scan(&sh.ID, &sh.Alias, &sh.Path, &perm, &en, &sh.CreatedAt, &sh.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	sh.Permission = model.SharePermission(perm)
	sh.Enabled = en == 1
	return &sh, nil
}

// boolToInt SQLite 用 INTEGER 存储布尔
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
