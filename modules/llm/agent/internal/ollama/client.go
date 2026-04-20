package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	llmcommon "github.com/gaspardpetit/nfrx/modules/llm/common"
)

// Client is a tiny HTTP client for talking to local Ollama.
type Client struct {
	BaseURL    string
	httpClient *http.Client
}

func New(base string) *Client {
	return &Client{BaseURL: base, httpClient: &http.Client{}}
}

func (c *Client) Tags(ctx context.Context) ([]string, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/tags", nil)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("tags status %s body=%q", resp.Status, summarizeBody(body, 256))
	}
	var v struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	var models []string
	for _, m := range v.Models {
		models = append(models, m.Name)
	}
	return models, nil
}

// Health checks the Ollama instance by fetching tags. Success indicates a
// healthy Ollama and returns the list of available models.
func (c *Client) Health(ctx context.Context) ([]string, error) {
	return c.Tags(ctx)
}

func (c *Client) GenerateStream(ctx context.Context, req llmcommon.GenerateRequest) (io.ReadCloser, error) {
	b, _ := json.Marshal(req)
	base := strings.TrimRight(c.BaseURL, "/")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/generate?stream=true", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) Generate(ctx context.Context, req llmcommon.GenerateRequest) ([]byte, error) {
	b, _ := json.Marshal(req)
	base := strings.TrimRight(c.BaseURL, "/")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/generate", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return io.ReadAll(resp.Body)
}

// ReadLines returns a channel streaming lines from reader.
func ReadLines(r io.Reader) <-chan string {
	ch := make(chan string)
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			ch <- scanner.Text()
		}
		close(ch)
	}()
	return ch
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
