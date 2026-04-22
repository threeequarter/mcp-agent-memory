package db

import (
	"context"
	"database/sql"
	"time"

	"memory-mcp/internal/vec"
)

type Fragment struct {
	ID        int64   `json:"id"`
	Domain    string  `json:"domain"`
	Content   string  `json:"content"`
	Score     float32 `json:"score"`
	CreatedAt string  `json:"created_at"`
}

func InsertFragment(ctx context.Context, db *sql.DB,
	domain, content string, embedding []float32) (int64, error) {

	res, err := db.ExecContext(ctx, `
		INSERT INTO fragments(domain, content, embedding, created_at)
		VALUES (?, ?, ?, ?)`,
		domain, content,
		vec.Pack(embedding), time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func SearchFragments(ctx context.Context, db *sql.DB,
	query []float32, domain string, k int) ([]Fragment, error) {

	var (
		rows *sql.Rows
		err  error
	)
	if domain != "" {
		rows, err = db.QueryContext(ctx,
			`SELECT id, domain, content, embedding, created_at
			 FROM fragments WHERE domain = ?`, domain)
	} else {
		rows, err = db.QueryContext(ctx,
			`SELECT id, domain, content, embedding, created_at FROM fragments`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type meta struct{ domain, content, createdAt string }
	var candidates []vec.Result
	metas := map[int64]meta{}

	for rows.Next() {
		var id int64
		var m meta
		var embedding []byte
		if err := rows.Scan(&id, &m.domain, &m.content,
			&embedding, &m.createdAt); err != nil {
			return nil, err
		}
		candidates = append(candidates, vec.Result{ID: id, Embedding: embedding})
		metas[id] = m
	}

	top := vec.TopK(query, candidates, k)
	result := make([]Fragment, 0, len(top))
	for _, r := range top {
		m := metas[r.ID]
		result = append(result, Fragment{
			ID: r.ID, Domain: m.domain,
			Content: m.content, Score: r.Score, CreatedAt: m.createdAt,
		})
	}
	return result, nil
}
