package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is a tiny HTTP client for talking to OpenAI-compatible endpoints.
type Client struct {
	BaseURL    string
	APIKey     string
	httpClient *http.Client
}

func New(base, apiKey string) *Client {
	return &Client{
		BaseURL:    base,
		APIKey:     apiKey,
		httpClient: &http.Client{},
	}
}

func (c *Client) Models(ctx context.Context) ([]string, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("models status %s body=%q", resp.Status, summarizeBody(body, 256))
	}
	var v struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	var models []string
	for _, m := range v.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

func summarizeBody(body []byte, limit int) string {
	if limit <= 0 || len(body) == 0 {
		return ""
	}
	s := strings.TrimSpace(string(body))
	if len(s) <= limit {
		return s
	}
	if limit <= 3 {
		return s[:limit]
	}
	return s[:limit-3] + "..."
}
