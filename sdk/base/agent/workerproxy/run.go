package workerproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gaspardpetit/nfrx/core/logx"
	reconnect "github.com/gaspardpetit/nfrx/core/reconnect"
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
	"github.com/gaspardpetit/nfrx/sdk/base/agent"
	dr "github.com/gaspardpetit/nfrx/sdk/base/agent/drain"
)

// Run starts the generic worker HTTP-proxy agent using the provided config.
func Run(ctx context.Context, cfg Config) error {
    if cfg.ClientID == "" {
        cfg.ClientID = time.Now().Format("20060102150405")
    }
    // Start advertising with zero concurrency until backend health is known
    SetWorkerInfo(cfg.ClientID, cfg.ClientName, 0)
    SetState("not_ready")
    SetConnectedToServer(false)
    // Start with backend unknown; probe will update
    SetConnectedToBackend(false)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Bridge shared drain state to local behavior and advertise transitions.
	statusUpdates := make(chan ctrl.StatusUpdateMessage, 16)
    dr.OnCheck(func() {
        if dr.IsDraining() {
            // Advertise draining with zero concurrency so the scheduler stops routing new work.
            SetState("draining")
            SetWorkerInfo(cfg.ClientID, cfg.ClientName, 0)
            su := ctrl.StatusUpdateMessage{Type: "status_update", Status: "draining", MaxConcurrency: 0}
            sendStatusUpdate(statusUpdates, su)
            triggerDrainCheck()
            return
        }
        s := GetState()
        newStatus := "disconnected"
        if s.ConnectedToServer {
            if s.CurrentJobs > 0 {
                newStatus = "connected_busy"
            } else {
                newStatus = "connected_idle"
            }
        }
        SetState(newStatus)
        su := ctrl.StatusUpdateMessage{Type: "status_update", Status: "idle", MaxConcurrency: s.MaxConcurrency}
        sendStatusUpdate(statusUpdates, su)
    })

	if cfg.StatusAddr != "" {
		if _, err := StartStatusServer(ctx, cfg.StatusAddr, cfg.TokenBasename, cfg.ConfigFile, cfg.DrainTimeout, cancel); err != nil {
			return err
		}
	}
	if cfg.MetricsAddr != "" {
		if _, err := StartMetricsServer(ctx, cfg.MetricsAddr); err != nil {
			return err
		}
	}

    // Probe backend health. Perform an initial synchronous probe (if provided)
    // so our first registration reflects accurate readiness/concurrency, then
    // continue probing periodically in the background. When no probe is
    // provided, assume the backend is ready and advertise the configured
    // concurrency to avoid permanently-disabled workers.
    interval := cfg.ProbeInterval
    if interval <= 0 { interval = 20 * time.Second }
    if cfg.ProbeFunc != nil {
        // Initial probe with short timeout to populate state before connecting
        _ = probeBackend(ctx, cfg, statusUpdates)
        startBackendMonitor(ctx, cfg, statusUpdates, interval)
    } else {
        // No probe configured: mark backend as ready with configured concurrency
        SetConnectedToBackend(true)
        SetWorkerInfo(cfg.ClientID, cfg.ClientName, cfg.MaxConcurrency)
        SetLabels(nil)
        SetLastError("")
    }

	return agent.RunWithReconnect(ctx, cfg.Reconnect, func(runCtx context.Context) error {
		SetState("connecting")
		SetConnectedToServer(false)
		_, err := connectAndServe(runCtx, cancel, cfg, statusUpdates)
		return err
	})
}

// Periodic health check for the upstream backend
func startBackendMonitor(ctx context.Context, cfg Config, ch chan<- ctrl.StatusUpdateMessage, interval time.Duration) {
    go monitorBackend(ctx, cfg, ch, interval)
}

func monitorBackend(ctx context.Context, cfg Config, ch chan<- ctrl.StatusUpdateMessage, interval time.Duration) {
	attempt := 0
	for {
		err := probeBackend(ctx, cfg, ch)
		if err != nil {
			if !cfg.Reconnect {
				return
			}
			delay := reconnect.Delay(attempt)
			attempt++
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		} else {
			attempt = 0
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	}
}

func probeBackend(ctx context.Context, cfg Config, ch chan<- ctrl.StatusUpdateMessage) error {
    // Custom probe hook takes precedence
    if cfg.ProbeFunc != nil {
        healthTO := 10 * time.Second
        if cfg.RequestTimeout > 0 && cfg.RequestTimeout < healthTO { healthTO = cfg.RequestTimeout }
        pctx, cancel := context.WithTimeout(ctx, healthTO)
        defer cancel()
        res, err := cfg.ProbeFunc(pctx)
        if err != nil || !res.Ready {
            was := GetState().ConnectedToBackend
            SetConnectedToBackend(false)
            SetWorkerInfo(cfg.ClientID, cfg.ClientName, 0)
            SetLabels(nil)
            SetState("not_ready")
            if err != nil { SetLastError(err.Error()) }
            if was { logx.Log.Warn().Err(err).Msg("backend probe failed; became not_ready") } else { logx.Log.Warn().Err(err).Msg("backend probe failed") }
            msg := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 0, Status: "not_ready"}
            sendStatusUpdate(ch, msg)
            return errIfNil(err, nil)
        }
        // Ready
        prevConnected := GetState().ConnectedToBackend
        prevModels := append([]string(nil), GetState().Labels...)
        prevMaxC := GetState().MaxConcurrency
        SetConnectedToBackend(true)
        maxc := cfg.MaxConcurrency
        if res.MaxConcurrency > 0 { maxc = res.MaxConcurrency }
        SetWorkerInfo(cfg.ClientID, cfg.ClientName, maxc)
        if len(res.Models) > 0 { SetLabels(res.Models) } else { SetLabels(nil) }
        if GetState().ConnectedToServer && !IsDraining() && GetState().CurrentJobs == 0 {
            SetState("connected_idle")
        }
        SetLastError("")
        // Notify on connectivity flip, models change, or concurrency change
        changed := !prevConnected
        if !changed {
            a := GetState().Labels; b := prevModels
            if len(a) != len(b) { changed = true } else {
                for i := range a { if a[i] != b[i] { changed = true; break } }
            }
        }
        if !changed && prevMaxC != maxc { changed = true }
        if changed {
            logx.Log.Info().Int("models", len(GetState().Labels)).Msg("backend ready")
            msg := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: GetState().MaxConcurrency, Models: GetState().Labels, Status: "idle"}
            sendStatusUpdate(ch, msg)
        }
        return nil
    }
    // No probe configured
    return nil
}

func errIfNil(err error, resp *http.Response) error {
	if err != nil {
		return err
	}
	if resp != nil {
		return fmt.Errorf("status %s", resp.Status)
	}
	return fmt.Errorf("probe failed")
}

func connectAndServe(ctx context.Context, cancelAll context.CancelFunc, cfg Config, statusUpdates <-chan ctrl.StatusUpdateMessage) (bool, error) {
	connCtx, cancelConn := context.WithCancel(ctx)
	ws, _, err := websocket.Dial(connCtx, cfg.ServerURL, nil)
	if err != nil {
		cancelConn()
		SetLastError(err.Error())
		SetState("error")
		return false, err
	}
	defer func() { _ = ws.Close(websocket.StatusInternalError, "closing") }()
	logx.Log.Info().Str("server", cfg.ServerURL).Msg("connected to server")
	SetConnectedToServer(true)
	SetState("connected_idle")
	SetLastError("")

    // Populate AgentConfig for extensible values
    agentCfg := map[string]string{}
    for k, v := range cfg.AgentConfig { agentCfg[k] = v }
    regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: cfg.ClientID, WorkerName: cfg.ClientName, ClientKey: cfg.ClientKey, Models: GetState().Labels, MaxConcurrency: GetState().MaxConcurrency, AgentConfig: agentCfg}
	b, _ := json.Marshal(regMsg)
	if err := ws.Write(connCtx, websocket.MessageText, b); err != nil {
		cancelConn()
		SetLastError(err.Error())
		SetState("error")
		return false, err
	}

	sendCh := make(chan []byte, 16)
	var senderWG sync.WaitGroup
	go func() {
		defer cancelConn()
		for {
			select {
			case msg, ok := <-sendCh:
				if !ok {
					return
				}
				if err := ws.Write(connCtx, websocket.MessageText, msg); err != nil {
					return
				}
			case <-connCtx.Done():
				return
			}
		}
	}()
	senderWG.Add(1)
	go func() {
		defer senderWG.Done()
        for {
            select {
            case su := <-statusUpdates:
                // Merge baseline agent config if not provided
                if su.AgentConfig == nil { su.AgentConfig = map[string]string{} }
                for k, v := range cfg.AgentConfig { if _, ok := su.AgentConfig[k]; !ok { su.AgentConfig[k] = v } }
                if len(su.Models) == 0 { su.Models = GetState().Labels }
                mb, _ := json.Marshal(su)
                sendMsg(connCtx, sendCh, mb)
            case <-connCtx.Done():
                return
            }
        }
    }()
	senderWG.Add(1)
	go func() {
		defer senderWG.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-connCtx.Done():
				return
			case t := <-ticker.C:
				hb := ctrl.HeartbeatMessage{Type: "heartbeat", TS: t.Unix()}
				bb, _ := json.Marshal(hb)
				sendMsg(connCtx, sendCh, bb)
				SetLastHeartbeat(t)
			}
		}
	}()

	checkDrain := func() {
		if connCtx.Err() != nil {
			return
		}
		if IsDraining() && GetState().CurrentJobs == 0 {
			SetState("terminating")
			logx.Log.Info().Msg("agent drained; closing connection")
			go func() { _ = ws.Close(websocket.StatusNormalClosure, "drained") }()
			cancelConn()
			cancelAll()
		}
	}
	setDrainCheck(checkDrain)
	defer setDrainCheck(nil)
	checkDrain()

	// Shared cancel registry for in-flight proxy requests
	reqCancels := make(map[string]context.CancelFunc)
	var jobMu sync.Mutex

	for {
		_, data, err := ws.Read(connCtx)
		if err != nil {
			SetConnectedToServer(false)
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				logx.Log.Error().Str("reason", ce.Reason).Msg("server connection closed")
			} else {
				logx.Log.Error().Err(err).Msg("server read error")
			}
			if IsDraining() {
				return true, nil
			}
			SetLastError(err.Error())
			SetState("error")
			return true, err
		}
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		switch env.Type {
		case "http_proxy_request":
			var hr ctrl.HTTPProxyRequestMessage
			if err := json.Unmarshal(data, &hr); err != nil {
				continue
			}
			if IsDraining() {
				h := ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: hr.RequestID, Status: 503, Headers: map[string]string{"Content-Type": "application/json"}}
				hb, _ := json.Marshal(h)
				sendMsg(connCtx, sendCh, hb)
				end := ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: hr.RequestID, Error: &ctrl.HTTPProxyError{Code: "worker_draining", Message: "worker is draining"}}
				eb, _ := json.Marshal(end)
				sendMsg(connCtx, sendCh, eb)
				continue
			}
			go handleHTTPProxy(connCtx, cfg, sendCh, hr, reqCancels, &jobMu, checkDrain)
		case "http_proxy_cancel":
			var hc ctrl.HTTPProxyCancelMessage
			if err := json.Unmarshal(data, &hc); err == nil {
				jobMu.Lock()
				if cancel, ok := reqCancels[hc.RequestID]; ok {
					cancel()
					delete(reqCancels, hc.RequestID)
				}
				jobMu.Unlock()
			}
		}
	}
}

func sendStatusUpdate(ch chan<- ctrl.StatusUpdateMessage, msg ctrl.StatusUpdateMessage) {
	if ch == nil {
		return
	}
	select {
	case ch <- msg:
	default:
	}
}
func sendMsg(ctx context.Context, ch chan<- []byte, msg []byte) { agent.Send(ctx, ch, msg) }
