package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // sqlite: single writer
	if err := initSchema(db); err != nil {
		return nil, err
	}
	return db, nil
}

func DBPath() string {
	if p := os.Getenv("AGENT_MEMORY_DATABASE"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return "memory.db"
	}
	return filepath.Join(filepath.Dir(exe), "memory.db")
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS profile (
			domain      TEXT PRIMARY KEY,
			content     TEXT NOT NULL,  -- JSON object: section_name → text
			updated_at  TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS fragments (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			domain      TEXT    NOT NULL,
			content     TEXT    NOT NULL,
			embedding   BLOB    NOT NULL,               -- float32 LE, 768 dims
			created_at  TEXT    NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_fragments_domain ON fragments(domain);

		CREATE TABLE IF NOT EXISTS episodes (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			domain      TEXT    NOT NULL,
			title       TEXT    NOT NULL,
			content     TEXT    NOT NULL,
			embedding   BLOB    NOT NULL,
			created_at  TEXT    NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_episodes_domain ON episodes(domain);
	`)
	return err
}
