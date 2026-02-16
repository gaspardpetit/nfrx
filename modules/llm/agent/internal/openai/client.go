package openai

import (
	"context"
	"encoding/json"
	"fmt"
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
		return nil, fmt.Errorf("models status %s", resp.Status)
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
