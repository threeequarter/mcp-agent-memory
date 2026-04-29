package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// GetProfile returns all sections of a domain profile.
// Section values may be strings, arrays, or nested objects.
// Returns an empty map if the domain does not exist.
func GetProfile(ctx context.Context, db *sql.DB,
	domain string) (map[string]any, error) {

	var content string
	err := db.QueryRowContext(ctx,
		"SELECT content FROM profile WHERE domain = ?", domain,
	).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result map[string]any
	return result, json.Unmarshal([]byte(content), &result)
}

// UpsertProfileSection replaces one section of a domain profile.
// The content string is parsed as JSON; if it parses as an array or
// object, the structured value is stored. Otherwise the raw string is
// stored as-is, preserving backward compatibility for plain-text values.
// Returns the previous content of the replaced section (nil if the
// section did not exist) so callers can report what was replaced.
func UpsertProfileSection(ctx context.Context, db *sql.DB,
	domain, section, content string) (any, error) {

	existing, err := GetProfile(ctx, db, domain)
	if err != nil {
		return nil, err
	}

	previous := existing[section]

	// Try to parse content as JSON. If it parses as an array or
	// object, store the structured value. Otherwise store the raw
	// string — this preserves backward compatibility for plain-text
	// section values and avoids treating JSON scalars ("true", "42")
	// as anything other than strings.
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		switch parsed.(type) {
		case []any, map[string]any:
			existing[section] = parsed
		default:
			existing[section] = content
		}
	} else {
		existing[section] = content
	}

	b, err := json.Marshal(existing)
	if err != nil {
		return previous, err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO profile(domain, content, updated_at) VALUES(?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			content    = excluded.content,
			updated_at = excluded.updated_at`,
		domain, string(b), time.Now().UTC().Format(time.RFC3339),
	)
	return previous, err
}
