package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	DefaultHost  = "http://localhost:11434"
	DefaultModel = "nomic-embed-text"
)

type Client struct {
	host  string
	model string
	http  *http.Client
}

func New(host, model string) *Client {
	if host == "" {
		host = DefaultHost
	}
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		host:  host,
		model: model,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(embedRequest{Model: c.model, Prompt: text})
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.host+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	var result embedResponse
	return result.Embedding, json.NewDecoder(resp.Body).Decode(&result)
}

func (c *Client) Unload(ctx context.Context) {
	body, _ := json.Marshal(map[string]any{
		"model":      c.model,
		"prompt":     "",
		"keep_alive": 0,
	})
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
