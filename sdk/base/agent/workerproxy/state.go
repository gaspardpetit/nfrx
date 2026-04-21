package workerproxy

import (
	"sync"
	"sync/atomic"
	"time"

	dr "github.com/gaspardpetit/nfrx/sdk/base/agent/drain"
)

type State struct {
	State              string            `json:"state"`
	ConnectedToServer  bool              `json:"connected_to_server"`
	ConnectedToBackend bool              `json:"connected_to_backend"`
	CurrentJobs        int               `json:"current_jobs"`
	MaxConcurrency     int               `json:"max_concurrency"`
	LastError          string            `json:"last_error"`
	LastHeartbeat      time.Time         `json:"last_heartbeat"`
	WorkerID           string            `json:"worker_id"`
	WorkerName         string            `json:"worker_name"`
	Version            string            `json:"version"`
	AgentConfig        map[string]string `json:"agent_config,omitempty"`
	Labels             []string          `json:"labels,omitempty"`
}

type VersionInfo struct{ Version, BuildSHA, BuildDate string }

var (
	stateMu    sync.RWMutex
	stateData  = State{State: "disconnected"}
	buildInfo  = VersionInfo{Version: "dev", BuildSHA: "unknown", BuildDate: "unknown"}
	draining   atomic.Bool
	drainMu    sync.Mutex
	drainCheck func()
)

func SetBuildInfo(v, sha, date string) {
	buildInfo = VersionInfo{Version: v, BuildSHA: sha, BuildDate: date}
	stateMu.Lock()
	stateData.Version = v
	stateMu.Unlock()
}
func GetVersionInfo() VersionInfo { return buildInfo }
func SetAgentConfig(cfg map[string]string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	if len(cfg) == 0 {
		stateData.AgentConfig = nil
		return
	}
	stateData.AgentConfig = cloneAgentConfig(cfg)
}
func MergeAgentConfig(cfg map[string]string) bool {
	if len(cfg) == 0 {
		return false
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	if stateData.AgentConfig == nil {
		stateData.AgentConfig = map[string]string{}
	}
	changed := false
	for k, v := range cfg {
		if cur, ok := stateData.AgentConfig[k]; !ok || cur != v {
			stateData.AgentConfig[k] = v
			changed = true
		}
	}
	return changed
}
func GetAgentConfig() map[string]string {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return cloneAgentConfig(stateData.AgentConfig)
}
func SetWorkerInfo(id, name string, maxConc int) {
	stateMu.Lock()
	stateData.WorkerID = id
	stateData.WorkerName = name
	stateData.MaxConcurrency = maxConc
	cur := stateData.CurrentJobs
	stateMu.Unlock()
	setCurrentJobs(cur)
}
func SetLabels(labels []string) {
	stateMu.Lock()
	stateData.Labels = append([]string(nil), labels...)
	stateMu.Unlock()
}
func SetState(s string)           { stateMu.Lock(); stateData.State = s; stateMu.Unlock() }
func SetConnectedToServer(v bool) { stateMu.Lock(); stateData.ConnectedToServer = v; stateMu.Unlock() }
func SetConnectedToBackend(v bool) {
	stateMu.Lock()
	stateData.ConnectedToBackend = v
	stateMu.Unlock()
}
func SetLastError(e string)        { stateMu.Lock(); stateData.LastError = e; stateMu.Unlock() }
func SetLastHeartbeat(t time.Time) { stateMu.Lock(); stateData.LastHeartbeat = t; stateMu.Unlock() }
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
	rem := stateData.CurrentJobs
	if rem == 0 && stateData.ConnectedToServer && !IsDraining() {
		stateData.State = "connected_idle"
	}
	stateMu.Unlock()
	setCurrentJobs(rem)
	return rem
}
func GetState() State { stateMu.RLock(); defer stateMu.RUnlock(); return stateData }

// metrics wiring handled elsewhere; no-op setter by default.
func JobStarted()                              {}
func JobCompleted(success bool, d interface{}) {}
func setCurrentJobs(int)                       {}

// Drain integration
func StartDrain() { draining.Store(true); SetState("draining"); dr.Start(); triggerDrainCheck() }
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
	dr.Stop()
}
func IsDraining() bool {
	if dr.IsDraining() != draining.Load() {
		draining.Store(dr.IsDraining())
	}
	return dr.IsDraining()
}
func setDrainCheck(fn func()) { drainMu.Lock(); drainCheck = fn; drainMu.Unlock() }
func triggerDrainCheck() {
	drainMu.Lock()
	fn := drainCheck
	drainMu.Unlock()
	if fn != nil {
		fn()
	}
}

func cloneAgentConfig(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
