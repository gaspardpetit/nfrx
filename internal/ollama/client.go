package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/you/llamapool/internal/relay"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
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

func (c *Client) GenerateStream(ctx context.Context, req relay.GenerateRequest) (io.ReadCloser, error) {
	b, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate?stream=true", bytes.NewReader(b))
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

func (c *Client) Generate(ctx context.Context, req relay.GenerateRequest) ([]byte, error) {
	b, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/generate", bytes.NewReader(b))
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
