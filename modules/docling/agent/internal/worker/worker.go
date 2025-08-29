package worker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"

	"fmt"
	"github.com/gaspardpetit/nfrx/core/logx"
	reconnect "github.com/gaspardpetit/nfrx/core/reconnect"
	aconfig "github.com/gaspardpetit/nfrx/modules/docling/agent/internal/config"
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
	"github.com/gaspardpetit/nfrx/sdk/base/agent"
	dr "github.com/gaspardpetit/nfrx/sdk/base/agent/drain"
	"net/http"
)

func Run(ctx context.Context, cfg aconfig.WorkerConfig) error {
	if cfg.ClientID == "" {
		cfg.ClientID = time.Now().Format("20060102150405")
	}
	SetWorkerInfo(cfg.ClientID, cfg.ClientName, cfg.MaxConcurrency)
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
			SetState("draining")
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
		if _, err := StartStatusServer(ctx, cfg.StatusAddr, cfg.ConfigFile, cfg.DrainTimeout, cancel); err != nil {
			return err
		}
	}
	if cfg.MetricsAddr != "" {
		if _, err := StartMetricsServer(ctx, cfg.MetricsAddr); err != nil {
			return err
		}
	}

	// Probe backend health periodically
	startBackendMonitor(ctx, cfg, statusUpdates, 20*time.Second)

	return agent.RunWithReconnect(ctx, cfg.Reconnect, func(runCtx context.Context) error {
		SetState("connecting")
		SetConnectedToServer(false)
		_, err := connectAndServe(runCtx, cancel, cfg, statusUpdates)
		return err
	})
}

// Periodic health check for docling backend
func startBackendMonitor(ctx context.Context, cfg aconfig.WorkerConfig, ch chan<- ctrl.StatusUpdateMessage, interval time.Duration) {
	go monitorBackend(ctx, cfg, ch, interval)
}

func monitorBackend(ctx context.Context, cfg aconfig.WorkerConfig, ch chan<- ctrl.StatusUpdateMessage, interval time.Duration) {
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

func probeBackend(ctx context.Context, cfg aconfig.WorkerConfig, ch chan<- ctrl.StatusUpdateMessage) error {
	// GET {BaseURL}/health
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.BaseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			_ = resp.Body.Close()
		}
		was := GetState().ConnectedToBackend
		SetConnectedToBackend(false)
		SetWorkerInfo(cfg.ClientID, cfg.ClientName, 0)
		SetState("not_ready")
		if err != nil {
			SetLastError(err.Error())
		} else {
			SetLastError(resp.Status)
		}
		if was {
			logx.Log.Warn().Str("status", func() string {
				if resp != nil {
					return resp.Status
				}
				return ""
			}()).Msg("docling backend probe failed; became not_ready")
		} else {
			logx.Log.Warn().Str("status", func() string {
				if resp != nil {
					return resp.Status
				}
				return ""
			}()).Msg("docling backend probe failed")
		}
		msg := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 0, Status: "not_ready"}
		sendStatusUpdate(ch, msg)
		return errIfNil(err, resp)
	}
	_ = resp.Body.Close()
	prev := GetState().ConnectedToBackend
	SetConnectedToBackend(true)
	SetWorkerInfo(cfg.ClientID, cfg.ClientName, cfg.MaxConcurrency)
	if GetState().ConnectedToServer && !IsDraining() && GetState().CurrentJobs == 0 {
		SetState("connected_idle")
	}
	SetLastError("")
	if !prev {
		logx.Log.Info().Msg("docling backend ready")
		msg := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: cfg.MaxConcurrency, Status: "idle"}
		sendStatusUpdate(ch, msg)
	}
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

func connectAndServe(ctx context.Context, cancelAll context.CancelFunc, cfg aconfig.WorkerConfig, statusUpdates <-chan ctrl.StatusUpdateMessage) (bool, error) {
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

	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: cfg.ClientID, WorkerName: cfg.ClientName, ClientKey: cfg.ClientKey, Models: nil, MaxConcurrency: cfg.MaxConcurrency, EmbeddingBatchSize: 0}
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
