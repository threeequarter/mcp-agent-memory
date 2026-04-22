package db

import (
	"context"
	"database/sql"
	"time"

	"memory-mcp/internal/vec"
)

type Episode struct {
	ID        int64   `json:"id"`
	Domain    string  `json:"domain"`
	Title     string  `json:"title"`
	Content   string  `json:"content"`
	Score     float32 `json:"score,omitempty"`
	CreatedAt string  `json:"created_at"`
}

func InsertEpisode(ctx context.Context, db *sql.DB,
	domain, title, content string, embedding []float32) (int64, error) {

	res, err := db.ExecContext(ctx, `
		INSERT INTO episodes(domain, title, content, embedding, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		domain, title, content,
		vec.Pack(embedding), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func SearchEpisodes(ctx context.Context, db *sql.DB,
	query []float32, domain string, k int) ([]Episode, error) {

	var (
		rows *sql.Rows
		err  error
	)
	if domain != "" {
		rows, err = db.QueryContext(ctx,
			`SELECT id, domain, title, content, embedding, created_at
			 FROM episodes WHERE domain = ?`, domain)
	} else {
		rows, err = db.QueryContext(ctx,
			`SELECT id, domain, title, content, embedding, created_at FROM episodes`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type meta struct{ domain, title, content, createdAt string }
	var candidates []vec.Result
	metas := map[int64]meta{}

	for rows.Next() {
		var id int64
		var m meta
		var embedding []byte
		if err := rows.Scan(&id, &m.domain, &m.title, &m.content,
			&embedding, &m.createdAt); err != nil {
			return nil, err
		}
		candidates = append(candidates, vec.Result{ID: id, Embedding: embedding})
		metas[id] = m
	}

	top := vec.TopK(query, candidates, k)
	result := make([]Episode, 0, len(top))
	for _, r := range top {
		m := metas[r.ID]
		result = append(result, Episode{
			ID: r.ID, Domain: m.domain, Title: m.title,
			Content: m.content, Score: r.Score, CreatedAt: m.createdAt,
		})
	}
	return result, nil
}

func ListEpisodes(ctx context.Context, db *sql.DB,
	domain string, limit int) ([]Episode, error) {

	var (
		rows *sql.Rows
		err  error
	)
	if domain != "" {
		rows, err = db.QueryContext(ctx, `
			SELECT id, domain, title, created_at FROM episodes
			WHERE domain = ? ORDER BY created_at DESC LIMIT ?`, domain, limit)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT id, domain, title, created_at FROM episodes
			ORDER BY created_at DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Episode
	for rows.Next() {
		var e Episode
		if err := rows.Scan(&e.ID, &e.Domain, &e.Title, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, nil
}
