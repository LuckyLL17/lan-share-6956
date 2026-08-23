// Package db 负责数据库连接、迁移与生命周期管理。
// 通过纯 Go 的 modernc.org/sqlite 驱动，避免 CGO 依赖。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	once sync.Once
	inst *sql.DB
)

// Open 打开或复用全局 SQLite 数据库连接。
// 多次调用线程安全，返回同一实例。
func Open(path string) (*sql.DB, error) {
	var err error
	once.Do(func() {
		d, e := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
		if e != nil {
			err = fmt.Errorf("open sqlite: %w", e)
			return
		}
		// 单连接足够本场景，避免锁竞争
		d.SetMaxOpenConns(1)
		d.SetMaxIdleConns(1)
		if e := d.PingContext(context.Background()); e != nil {
			err = fmt.Errorf("ping sqlite: %w", e)
			_ = d.Close()
			return
		}
		inst = d
		err = migrate(inst)
	})
	return inst, err
}

// Get 返回已打开的数据库实例，未打开时为 nil。
func Get() *sql.DB { return inst }

// Close 关闭数据库连接。
func Close() error {
	if inst == nil {
		return nil
	}
	return inst.Close()
}

// migrate 执行表结构的创建与索引初始化。
// 使用 IF NOT EXISTS 保证幂等。
func migrate(d *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			ip           TEXT NOT NULL,
			port         INTEGER NOT NULL DEFAULT 8765,
			status       TEXT NOT NULL DEFAULT 'online',
			first_seen   DATETIME NOT NULL,
			last_seen    DATETIME NOT NULL,
			os           TEXT DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(status);`,

		`CREATE TABLE IF NOT EXISTS shares (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			alias       TEXT NOT NULL UNIQUE,
			path        TEXT NOT NULL,
			permission  TEXT NOT NULL DEFAULT 'readonly',
			enabled     INTEGER NOT NULL DEFAULT 1,
			created_at  DATETIME NOT NULL,
			updated_at  DATETIME NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS transfers (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			direction         TEXT NOT NULL,
			peer_device_id     TEXT NOT NULL DEFAULT '',
			peer_ip           TEXT NOT NULL,
			peer_port         INTEGER NOT NULL DEFAULT 8765,
			share_alias       TEXT DEFAULT '',
			remote_path       TEXT DEFAULT '',
			local_path        TEXT DEFAULT '',
			file_name         TEXT NOT NULL,
			file_size         INTEGER NOT NULL DEFAULT 0,
			bytes_transferred INTEGER NOT NULL DEFAULT 0,
			status            TEXT NOT NULL DEFAULT 'queued',
			err_msg           TEXT DEFAULT '',
			created_at        DATETIME NOT NULL,
			updated_at        DATETIME NOT NULL,
			finished_at       DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_status ON transfers(status);`,
		`CREATE INDEX IF NOT EXISTS idx_transfers_created ON transfers(created_at DESC);`,

		`CREATE TABLE IF NOT EXISTS messages (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			from_ip     TEXT NOT NULL,
			from_name   TEXT NOT NULL DEFAULT '',
			to_ip       TEXT DEFAULT '',
			content     TEXT NOT NULL,
			read        INTEGER NOT NULL DEFAULT 0,
			created_at  DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created ON messages(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_read ON messages(read);`,
	}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			return fmt.Errorf("migrate stmt failed: %w\nSQL: %s", err, s)
		}
	}
	return nil
}
