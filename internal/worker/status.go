package worker

import (
	"sync"
	"sync/atomic"
	"time"
)

type State struct {
	State             string    `json:"state"`
	ConnectedToServer bool      `json:"connected_to_server"`
	ConnectedToOllama bool      `json:"connected_to_ollama"`
	CurrentJobs       int       `json:"current_jobs"`
	MaxConcurrency    int       `json:"max_concurrency"`
	Models            []string  `json:"models"`
	LastError         string    `json:"last_error"`
	LastHeartbeat     time.Time `json:"last_heartbeat"`
	WorkerID          string    `json:"worker_id"`
	WorkerName        string    `json:"worker_name"`
	Version           string    `json:"version"`
}

type VersionInfo struct {
	Version   string `json:"version"`
	BuildSHA  string `json:"build_sha"`
	BuildDate string `json:"build_date"`
}

var (
	stateMu   sync.RWMutex
	stateData = State{State: "disconnected"}
	buildInfo = VersionInfo{Version: "dev", BuildSHA: "unknown", BuildDate: "unknown"}
	draining  atomic.Bool
)

func resetState() {
	stateMu.Lock()
	defer stateMu.Unlock()
	stateData = State{State: "disconnected"}
	draining.Store(false)
}

func SetBuildInfo(v, sha, date string) {
	buildInfo = VersionInfo{Version: v, BuildSHA: sha, BuildDate: date}
	stateMu.Lock()
	stateData.Version = v
	stateMu.Unlock()
}

func GetVersionInfo() VersionInfo {
	return buildInfo
}

func SetWorkerInfo(id, name string, maxConc int, models []string) {
	stateMu.Lock()
	stateData.WorkerID = id
	stateData.WorkerName = name
	stateData.MaxConcurrency = maxConc
	stateData.Models = append([]string(nil), models...)
	cur := stateData.CurrentJobs
	stateMu.Unlock()
	setMaxConcurrency(maxConc)
	setCurrentJobs(cur)
}

func SetState(s string) {
	stateMu.Lock()
	stateData.State = s
	stateMu.Unlock()
}

func SetConnectedToServer(v bool) {
	stateMu.Lock()
	stateData.ConnectedToServer = v
	stateMu.Unlock()
	setConnectedToServer(v)
}

func SetConnectedToOllama(v bool) {
	stateMu.Lock()
	stateData.ConnectedToOllama = v
	stateMu.Unlock()
	setConnectedToOllama(v)
}

func SetModels(models []string) {
	stateMu.Lock()
	stateData.Models = append([]string(nil), models...)
	stateMu.Unlock()
}

func SetLastError(err string) {
	stateMu.Lock()
	stateData.LastError = err
	stateMu.Unlock()
}

func SetLastHeartbeat(t time.Time) {
	stateMu.Lock()
	stateData.LastHeartbeat = t
	stateMu.Unlock()
}

func IncJobs() {
	stateMu.Lock()
	stateData.CurrentJobs++
	if stateData.ConnectedToServer && !IsDraining() {
		stateData.State = "connected_busy"
	}
	cur := stateData.CurrentJobs
	stateMu.Unlock()
	setCurrentJobs(cur)
}

func DecJobs() int {
	stateMu.Lock()
	if stateData.CurrentJobs > 0 {
		stateData.CurrentJobs--
	}
	remaining := stateData.CurrentJobs
	if remaining == 0 && stateData.ConnectedToServer && !IsDraining() {
		stateData.State = "connected_idle"
	}
	stateMu.Unlock()
	setCurrentJobs(remaining)
	return remaining
}

func GetState() State {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return stateData
}

func StartDrain() {
	draining.Store(true)
	SetState("draining")
}

func StopDrain() {
	draining.Store(false)
	stateMu.Lock()
	if stateData.ConnectedToServer {
		if stateData.CurrentJobs > 0 {
			stateData.State = "connected_busy"
		} else {
			stateData.State = "connected_idle"
		}
	} else {
		stateData.State = "disconnected"
	}
	stateMu.Unlock()
}

func IsDraining() bool {
	return draining.Load()
}
