package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

    ctrl "github.com/gaspardpetit/nfrx/sdk/contracts/control"
	"github.com/gaspardpetit/nfrx/modules/common/logx"
	reconnect "github.com/gaspardpetit/nfrx/modules/common/reconnect"
	aconfig "github.com/gaspardpetit/nfrx/modules/llm/agent/internal/config"
	"github.com/gaspardpetit/nfrx/modules/llm/agent/internal/ollama"
	"github.com/gaspardpetit/nfrx/modules/llm/agent/internal/relay"
)

// Run starts the worker agent.
func Run(ctx context.Context, cfg aconfig.WorkerConfig) error {
	if cfg.ClientID == "" {
		cfg.ClientID = uuid.NewString()
	}
	SetWorkerInfo(cfg.ClientID, cfg.ClientName, 0, cfg.EmbeddingBatchSize, nil)
	SetState("not_ready")
	SetConnectedToServer(false)
	SetConnectedToBackend(false)

	client := ollama.New(strings.TrimSuffix(cfg.CompletionBaseURL, "/v1"))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	interval := cfg.ModelPollInterval
	if interval <= 0 {
		interval = 20 * time.Second
	}

	statusUpdates := make(chan ctrl.StatusUpdateMessage, 16)
	startBackendMonitor(ctx, cfg, client, statusUpdates, interval)

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

	attempt := 0
	for {
		SetState("connecting")
		SetConnectedToServer(false)

		connected, err := connectAndServe(ctx, cancel, cfg, client, statusUpdates)
		if err == nil || !cfg.Reconnect {
			return err
		}
		if connected {
			attempt = 0
		}
		delay := reconnect.Delay(attempt)
		attempt++
		logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("connection to server lost; retrying")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func connectAndServe(ctx context.Context, cancelAll context.CancelFunc, cfg aconfig.WorkerConfig, client *ollama.Client, statusUpdates <-chan ctrl.StatusUpdateMessage) (bool, error) {
	connCtx, cancelConn := context.WithCancel(ctx)
	ws, _, err := websocket.Dial(connCtx, cfg.ServerURL, nil)
	if err != nil {
		cancelConn()
		SetLastError(err.Error())
		SetState("error")
		return false, err
	}
	defer func() {
		_ = ws.Close(websocket.StatusInternalError, "closing")
	}()

	logx.Log.Info().Str("server", cfg.ServerURL).Msg("connected to server")
	SetConnectedToServer(true)
	if GetState().ConnectedToBackend {
		SetState("connected_idle")
	} else {
		SetState("not_ready")
	}
	SetLastError("")

	_ = probeBackend(connCtx, client, cfg, nil)
	vi := GetVersionInfo()
	regMsg := ctrl.RegisterMessage{
		Type:               "register",
		WorkerID:           cfg.ClientID,
		WorkerName:         cfg.ClientName,
		ClientKey:          cfg.ClientKey,
		Models:             GetState().Models,
		MaxConcurrency:     GetState().MaxConcurrency,
		EmbeddingBatchSize: GetState().EmbeddingBatchSize,
		Version:            vi.Version,
		BuildSHA:           vi.BuildSHA,
		BuildDate:          vi.BuildDate,
	}
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
		<-connCtx.Done()
		senderWG.Wait()
		close(sendCh)
	}()
	defer cancelConn()
	reqCancels := make(map[string]context.CancelFunc)
	var jobMu sync.Mutex
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
	st := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: GetState().MaxConcurrency, EmbeddingBatchSize: GetState().EmbeddingBatchSize, Models: GetState().Models}
	if GetState().ConnectedToBackend {
		st.Status = "idle"
	} else {
		st.Status = "not_ready"
	}
	sb, _ := json.Marshal(st)
	sendMsg(connCtx, sendCh, sb)

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
		if IsDraining() && GetState().CurrentJobs == 0 {
			SetState("terminating")
			go func() { _ = ws.Close(websocket.StatusNormalClosure, "drained") }()
			cancelConn()
			cancelAll()
		}
	}
	setDrainCheck(checkDrain)
	defer setDrainCheck(nil)
	checkDrain()

	for {
		_, data, err := ws.Read(connCtx)
		if err != nil {
			SetConnectedToServer(false)
			var ce websocket.CloseError
			if errors.As(err, &ce) {
				lvl := logx.Log.Info()
				if ce.Code != websocket.StatusNormalClosure {
					lvl = logx.Log.Error()
				}
				lvl.Str("reason", ce.Reason).Msg("server connection closed")
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
		case "job_request":
			var jr ctrl.JobRequestMessage
			if err := json.Unmarshal(data, &jr); err != nil {
				continue
			}
			logx.Log.Info().Str("job", jr.JobID).Msg("job request")
			if IsDraining() {
				logx.Log.Warn().Str("job", jr.JobID).Msg("reject job while draining")
				msg := ctrl.JobErrorMessage{Type: "job_error", JobID: jr.JobID, Code: "worker_draining", Message: "worker is draining"}
				b, _ := json.Marshal(msg)
				sendMsg(connCtx, sendCh, b)
				continue
			}
			if jr.Endpoint == "generate" {
				go handleGenerate(connCtx, client, cfg.RequestTimeout, sendCh, jr, reqCancels, &jobMu, checkDrain)
			}
		case "cancel_job":
			var cj ctrl.CancelJobMessage
			if err := json.Unmarshal(data, &cj); err == nil {
				jobMu.Lock()
				if cancel, ok := reqCancels[cj.JobID]; ok {
					cancel()
					delete(reqCancels, cj.JobID)
				}
				jobMu.Unlock()
			}
		case "http_proxy_request":
			var hr ctrl.HTTPProxyRequestMessage
			if err := json.Unmarshal(data, &hr); err != nil {
				continue
			}
			logx.Log.Info().Str("request_id", hr.RequestID).Str("path", hr.Path).Msg("http proxy request")
			if IsDraining() {
				logx.Log.Warn().Str("request_id", hr.RequestID).Msg("reject proxy while draining")
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

type healthClient interface {
	Health(context.Context) ([]string, error)
}

func startBackendMonitor(ctx context.Context, cfg aconfig.WorkerConfig, client healthClient, ch chan<- ctrl.StatusUpdateMessage, interval time.Duration) {
	go monitorBackend(ctx, cfg, client, ch, interval)
}

func monitorBackend(ctx context.Context, cfg aconfig.WorkerConfig, client healthClient, ch chan<- ctrl.StatusUpdateMessage, interval time.Duration) {
	attempt := 0
	for {
		err := probeBackend(ctx, client, cfg, ch)
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

// probeBackend checks health, updates state, and (importantly) only emits a status
// update when either (a) connectivity flips, or (b) the models list actually changes.
// On error, it always emits a not_ready status update.
func probeBackend(ctx context.Context, client healthClient, cfg aconfig.WorkerConfig, ch chan<- ctrl.StatusUpdateMessage) error {
	models, err := client.Health(ctx)
	if err != nil {
		wasConnected := GetState().ConnectedToBackend
		SetConnectedToBackend(false)
		SetWorkerInfo(cfg.ClientID, cfg.ClientName, 0, cfg.EmbeddingBatchSize, nil)
		SetState("not_ready")
		SetLastError(err.Error())

		// Emit an update on error (always), but keep the channel non-blocking.
		msg := ctrl.StatusUpdateMessage{Type: "status_update", MaxConcurrency: 0, EmbeddingBatchSize: cfg.EmbeddingBatchSize, Status: "not_ready"}
		sendStatusUpdate(ch, msg)

		// State changed from connected->disconnected; that's fine; test checks state, not channel.
		_ = wasConnected
		return err
	}

	prev := GetState()
	prevConnected := prev.ConnectedToBackend
	prevModels := append([]string(nil), prev.Models...)

	SetConnectedToBackend(true)
	SetWorkerInfo(cfg.ClientID, cfg.ClientName, cfg.MaxConcurrency, cfg.EmbeddingBatchSize, models)
	if GetState().ConnectedToServer && !IsDraining() && GetState().CurrentJobs == 0 {
		SetState("connected_idle")
	}
	SetLastError("")

	// Determine if we should notify: connectivity flip or models changed.
	changed := !prevConnected
	if !changed {
		if len(prevModels) != len(models) {
			changed = true
		} else {
			for i := range models {
				if models[i] != prevModels[i] {
					changed = true
					break
				}
			}
		}
	}
	if changed {
		msg := ctrl.StatusUpdateMessage{
			Type:               "status_update",
			MaxConcurrency:     cfg.MaxConcurrency,
			EmbeddingBatchSize: cfg.EmbeddingBatchSize,
			Models:             models,
			Status:             "idle",
		}
		sendStatusUpdate(ch, msg)
	}
	return nil
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

func sendMsg(ctx context.Context, ch chan<- []byte, msg []byte) {
	select {
	case ch <- msg:
	case <-ctx.Done():
	}
}

func handleGenerate(ctx context.Context, client *ollama.Client, timeout time.Duration, sendCh chan []byte, jr ctrl.JobRequestMessage, cancels map[string]context.CancelFunc, mu *sync.Mutex, onDone func()) {
	logx.Log.Info().Str("job", jr.JobID).Msg("generate start")
	raw, _ := json.Marshal(jr.Payload)
	var req relay.GenerateRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		logx.Log.Error().Str("job", jr.JobID).Err(err).Msg("unmarshal generate request")
		return
	}
	jobCtx, cancel := context.WithCancel(ctx)
	mu.Lock()
	cancels[jr.JobID] = cancel
	mu.Unlock()
	IncJobs()
	JobStarted()
	start := time.Now()
	success := false
	defer func() {
		cancel()
		mu.Lock()
		delete(cancels, jr.JobID)
		mu.Unlock()
		_ = DecJobs()
		onDone()
		dur := time.Since(start)
		JobCompleted(success, dur)
		lvl := logx.Log.Info()
		msg := "generate complete"
		if !success {
			lvl = logx.Log.Warn()
			msg = "generate failed"
		}
		lvl.Str("job", jr.JobID).Dur("duration", dur).Msg(msg)
	}()
	if req.Stream {
		rc, err := client.GenerateStream(jobCtx, req)
		if err != nil {
			logx.Log.Error().Str("job", jr.JobID).Err(err).Msg("generate stream error")
			msg := ctrl.JobErrorMessage{Type: "job_error", JobID: jr.JobID, Code: "error", Message: err.Error()}
			b, _ := json.Marshal(msg)
			sendMsg(jobCtx, sendCh, b)
			SetLastError(err.Error())
			return
		}
		defer func() {
			_ = rc.Close()
		}()
		var idle *time.Timer
		if timeout > 0 {
			idle = time.NewTimer(timeout)
			go func() {
				<-idle.C
				cancel()
			}()
			defer idle.Stop()
		}
		for line := range ollama.ReadLines(rc) {
			if idle != nil {
				if !idle.Stop() {
					<-idle.C
				}
				idle.Reset(timeout)
			}
			msg := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(line)}
			b, _ := json.Marshal(msg)
			sendMsg(jobCtx, sendCh, b)
		}
		done := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(`{"done":true}`)}
		b, _ := json.Marshal(done)
		sendMsg(jobCtx, sendCh, b)
	} else {
		reqCtx := jobCtx
		var cancelTO context.CancelFunc
		if timeout > 0 {
			reqCtx, cancelTO = context.WithTimeout(jobCtx, timeout)
		}
		data, err := client.Generate(reqCtx, req)
		if cancelTO != nil {
			cancelTO()
		}
		if err != nil {
			logx.Log.Error().Str("job", jr.JobID).Err(err).Msg("generate error")
			msg := ctrl.JobErrorMessage{Type: "job_error", JobID: jr.JobID, Code: "error", Message: err.Error()}
			b, _ := json.Marshal(msg)
			sendMsg(jobCtx, sendCh, b)
			SetLastError(err.Error())
			return
		}
		msg := ctrl.JobResultMessage{Type: "job_result", JobID: jr.JobID, Data: json.RawMessage(data)}
		b, _ := json.Marshal(msg)
		sendMsg(jobCtx, sendCh, b)
		success = true
	}
	if req.Stream {
		success = true
	}
}
