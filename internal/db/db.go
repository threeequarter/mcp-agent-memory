package db

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

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
	if err := seedDefaults(db); err != nil {
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
			content     TEXT NOT NULL,  -- JSON object: section_name → any (string, array, or object)
			updated_at  TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS fragments (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			domain      TEXT    NOT NULL,
			content     TEXT    NOT NULL,
			embedding   BLOB    NOT NULL,               -- float32 LE, dims depend on model
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

// seedDefaults populates an empty database with starter user and agent
// profiles. This gives a fresh installation structure that the agent can
// immediately fill in via profile_replace_section, rather than starting
// with a blank slate where it doesn't know what sections exist.
func seedDefaults(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM profile").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)

	defaults := map[string]map[string]string{
		"user": {
			"Identity":            "Name, age, profession, key interests.",
			"Communication Style": "Communication preferences: directness, formality, preferred formats.",
			"Domain Focus":        "Domains and their focus areas.",
		},
		"agent": {
			"Behavioral Boundaries":               "Rules and constraints for agent behavior.",
			"Communication Structure Constraints": "Sentence structure, formatting, style preferences.",
			"Vocabulary Constraints":              "Words and phrases to avoid or prefer.",
		},
	}

	for domain, sections := range defaults {
		b, err := json.Marshal(sections)
		if err != nil {
			return err
		}
		_, err = db.Exec(
			"INSERT INTO profile(domain, content, updated_at) VALUES(?, ?, ?)",
			domain, string(b), now,
		)
		if err != nil {
			return err
		}
	}

	log.Printf("seeded default user and agent profiles")
	return nil
}
