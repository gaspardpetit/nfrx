package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const backendVersionProbeTimeout = 2 * time.Second

type backendInfo struct {
	Family  string
	Version string
}

type propsResponse struct {
	BuildInfo string `json:"build_info"`
}

type apiVersionResponse struct {
	Version string `json:"version"`
}

func discoverBackendInfo(ctx context.Context, baseURL, apiKey string) backendInfo {
	root := strings.TrimRight(parentBase(normalizeBase(baseURL)), "/")
	if root == "" {
		return backendInfo{}
	}
	client := &http.Client{}
	if buildInfo := func() string {
		probeCtx, cancel := probeContext(ctx)
		defer cancel()
		return fetchBackendBuildInfo(probeCtx, client, root+"/props", apiKey)
	}(); buildInfo != "" {
		return backendInfo{Family: "llama.cpp", Version: buildInfo}
	}
	if version := func() string {
		probeCtx, cancel := probeContext(ctx)
		defer cancel()
		return fetchBackendAPIVersion(probeCtx, client, root+"/api/version", apiKey)
	}(); version != "" {
		return backendInfo{Family: "ollama", Version: version}
	}
	return backendInfo{}
}

func backendInfoFromOverride(version string) backendInfo {
	version = strings.TrimSpace(version)
	if version == "" {
		return backendInfo{}
	}
	return backendInfo{
		Family:  inferBackendFamily(version),
		Version: version,
	}
}

func inferBackendFamily(version string) string {
	version = strings.ToLower(strings.TrimSpace(version))
	switch {
	case strings.HasPrefix(version, "llama.cpp"):
		return "llama.cpp"
	case strings.HasPrefix(version, "ollama"):
		return "ollama"
	default:
		return "unknown"
	}
}

func backendAgentConfig(info backendInfo) map[string]string {
	if strings.TrimSpace(info.Version) == "" {
		return nil
	}
	cfg := map[string]string{
		"backend_version": strings.TrimSpace(info.Version),
	}
	family := strings.TrimSpace(info.Family)
	if family == "" {
		family = "unknown"
	}
	cfg["backend_family"] = family
	return cfg
}

func shouldDiscoverBackendInfo(agentConfig map[string]string) bool {
	return strings.TrimSpace(agentConfig["backend_version"]) == ""
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
