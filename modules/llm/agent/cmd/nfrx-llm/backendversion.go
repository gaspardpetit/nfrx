package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const backendVersionProbeTimeout = 2 * time.Second

type propsResponse struct {
	BuildInfo string `json:"build_info"`
}

type apiVersionResponse struct {
	Version string `json:"version"`
}

func discoverCompletionAgentVersion(ctx context.Context, baseURL, apiKey string) string {
	root := strings.TrimRight(parentBase(normalizeBase(baseURL)), "/")
	if root == "" {
		return ""
	}
	client := &http.Client{}
	if buildInfo := func() string {
		probeCtx, cancel := probeContext(ctx)
		defer cancel()
		return fetchBackendBuildInfo(probeCtx, client, root+"/props", apiKey)
	}(); buildInfo != "" {
		return "llama.cpp " + buildInfo
	}
	if version := func() string {
		probeCtx, cancel := probeContext(ctx)
		defer cancel()
		return fetchBackendAPIVersion(probeCtx, client, root+"/api/version", apiKey)
	}(); version != "" {
		return "ollama " + version
	}
	return ""
}

func probeContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, backendVersionProbeTimeout)
}

func fetchBackendBuildInfo(ctx context.Context, client *http.Client, url, apiKey string) string {
	var resp propsResponse
	if !fetchBackendJSON(ctx, client, url, apiKey, &resp) {
		return ""
	}
	return strings.TrimSpace(resp.BuildInfo)
}

func fetchBackendAPIVersion(ctx context.Context, client *http.Client, url, apiKey string) string {
	var resp apiVersionResponse
	if !fetchBackendJSON(ctx, client, url, apiKey, &resp) {
		return ""
	}
	return strings.TrimSpace(resp.Version)
}

func fetchBackendJSON(ctx context.Context, client *http.Client, url, apiKey string, out any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(out) == nil
}
