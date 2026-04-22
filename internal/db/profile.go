package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func GetProfile(ctx context.Context, db *sql.DB,
	domain string) (map[string]string, error) {

	var content string
	err := db.QueryRowContext(ctx,
		"SELECT content FROM profile WHERE domain = ?", domain,
	).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result map[string]string
	return result, json.Unmarshal([]byte(content), &result)
}

func UpsertProfileSection(ctx context.Context, db *sql.DB,
	domain, section, content string) error {

	existing, err := GetProfile(ctx, db, domain)
	if err != nil {
		return err
	}
	existing[section] = content
	b, err := json.Marshal(existing)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO profile(domain, content, updated_at) VALUES(?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			content    = excluded.content,
			updated_at = excluded.updated_at`,
		domain, string(b), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}
