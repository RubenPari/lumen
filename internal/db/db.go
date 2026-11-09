// Package db holds the SQLite storage layer.
package db

import (
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB with typed helpers.
type DB struct {
	*sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL,
	is_admin INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	device TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	last_seen INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS libraries (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	paths TEXT NOT NULL DEFAULT '[]',
	created_at INTEGER NOT NULL,
	last_scan INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS items (
	id TEXT PRIMARY KEY,
	library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
	type TEXT NOT NULL,
	parent_id TEXT,
	series_id TEXT,
	season_id TEXT,
	name TEXT NOT NULL,
	original_title TEXT NOT NULL DEFAULT '',
	sort_name TEXT NOT NULL DEFAULT '',
	path TEXT NOT NULL,
	is_folder INTEGER NOT NULL DEFAULT 0,
	year INTEGER NOT NULL DEFAULT 0,
	overview TEXT NOT NULL DEFAULT '',
	tagline TEXT NOT NULL DEFAULT '',
	premiere_date TEXT NOT NULL DEFAULT '',
	end_date TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT '',
	community_rating REAL NOT NULL DEFAULT 0,
	official_rating TEXT NOT NULL DEFAULT '',
	duration REAL NOT NULL DEFAULT 0,
	season_number INTEGER,
	episode_number INTEGER,
	episode_end INTEGER,
	container TEXT NOT NULL DEFAULT '',
	size INTEGER NOT NULL DEFAULT 0,
	mtime INTEGER NOT NULL DEFAULT 0,
	genres TEXT NOT NULL DEFAULT '[]',
	studios TEXT NOT NULL DEFAULT '[]',
	people TEXT NOT NULL DEFAULT '[]',
	tmdb_id TEXT NOT NULL DEFAULT '',
	imdb_id TEXT NOT NULL DEFAULT '',
	img_primary TEXT NOT NULL DEFAULT '',
	img_backdrop TEXT NOT NULL DEFAULT '',
	img_logo TEXT NOT NULL DEFAULT '',
	img_thumb TEXT NOT NULL DEFAULT '',
	media TEXT NOT NULL DEFAULT '{}',
	subtitles TEXT NOT NULL DEFAULT '[]',
	extra_type TEXT NOT NULL DEFAULT '',
	metadata_locked INTEGER NOT NULL DEFAULT 0,
	metadata_fetched INTEGER NOT NULL DEFAULT 0,
	date_added INTEGER NOT NULL,
	date_updated INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_items_library ON items(library_id, type);
CREATE INDEX IF NOT EXISTS idx_items_parent ON items(parent_id);
CREATE INDEX IF NOT EXISTS idx_items_series ON items(series_id);
CREATE INDEX IF NOT EXISTS idx_items_added ON items(date_added);
CREATE UNIQUE INDEX IF NOT EXISTS idx_items_path ON items(library_id, type, path);
CREATE TABLE IF NOT EXISTS user_data (
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	item_id TEXT NOT NULL,
	position REAL NOT NULL DEFAULT 0,
	played INTEGER NOT NULL DEFAULT 0,
	play_count INTEGER NOT NULL DEFAULT 0,
	favorite INTEGER NOT NULL DEFAULT 0,
	last_played INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, item_id)
);
CREATE INDEX IF NOT EXISTS idx_userdata_last ON user_data(user_id, last_played);
CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

// Open opens (and migrates) the database at path.
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)", path)
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sdb.SetMaxOpenConns(8)
	if _, err := sdb.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{sdb}, nil
}

// NewID returns a random 32 hex chars identifier.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// StableID derives a deterministic ID from parts (used for items so that
// rescans keep identifiers and user data stable).
func StableID(parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:16])
}

// Now returns the current unix time.
func Now() int64 { return time.Now().Unix() }

// GetSetting returns a setting value or def.
func (d *DB) GetSetting(key, def string) string {
	var v string
	if err := d.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// SetSetting upserts a setting.
func (d *DB) SetSetting(key, value string) error {
	_, err := d.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
