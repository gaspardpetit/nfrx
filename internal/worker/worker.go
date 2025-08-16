package worker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
	"github.com/gaspardpetit/llamapool/internal/logx"
	"github.com/gaspardpetit/llamapool/internal/ollama"
	"github.com/gaspardpetit/llamapool/internal/relay"
)

// Run starts the worker agent.
func Run(ctx context.Context, cfg config.WorkerConfig) error {
	if cfg.WorkerID == "" {
		cfg.WorkerID = uuid.NewString()
	}
	SetWorkerInfo(cfg.WorkerID, cfg.WorkerName, cfg.MaxConcurrency, nil)
	SetState("connecting")
	SetConnectedToServer(false)
	SetConnectedToOllama(false)

	client := ollama.New(cfg.OllamaBaseURL)
	models, err := client.Tags(ctx)
	if err != nil {
		SetLastError(err.Error())
		return err
	}
	SetModels(models)
	SetConnectedToOllama(true)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	interval := cfg.ModelPollInterval
	if interval <= 0 {
		interval = time.Minute
	}
	modelsCh := make(chan []string, 1)
	go startHealthProbe(ctx, client, interval, modelsCh)

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
		connected, err := connectAndServe(ctx, cfg, client, modelsCh)
		if err == nil || !cfg.Reconnect {
			return err
		}
		if connected {
			attempt = 0
		}
		delay := reconnectDelay(attempt)
		attempt++
		logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("connection to server lost; retrying")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func connectAndServe(ctx context.Context, cfg config.WorkerConfig, client *ollama.Client, modelsCh <-chan []string) (bool, error) {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ws, _, err := websocket.Dial(connCtx, cfg.ServerURL, nil)
	if err != nil {
		SetLastError(err.Error())
		SetState("error")
		return false, err
	}
	defer func() {
		_ = ws.Close(websocket.StatusInternalError, "closing")
	}()

	logx.Log.Info().Str("server", cfg.ServerURL).Msg("connected to server")
	SetConnectedToServer(true)
	SetState("connected_idle")
	SetLastError("")

	sendCh := make(chan []byte, 16)
	defer close(sendCh)
	reqCancels := make(map[string]context.CancelFunc)
	var jobMu sync.Mutex
	go func() {
		defer cancel()
		for msg := range sendCh {
			if err := ws.Write(connCtx, websocket.MessageText, msg); err != nil {
				return
			}
		}
	}()

	vi := GetVersionInfo()
	regMsg := ctrl.RegisterMessage{
		Type:           "register",
		WorkerID:       cfg.WorkerID,
		WorkerName:     cfg.WorkerName,
		WorkerKey:      cfg.WorkerKey,
		Models:         GetState().Models,
		MaxConcurrency: cfg.MaxConcurrency,
		Version:        vi.Version,
		BuildSHA:       vi.BuildSHA,
		BuildDate:      vi.BuildDate,
	}
	b, _ := json.Marshal(regMsg)
	sendCh <- b

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-connCtx.Done():
				return
			case t := <-ticker.C:
				hb := ctrl.HeartbeatMessage{Type: "heartbeat", TS: t.Unix()}
				bb, _ := json.Marshal(hb)
				sendCh <- bb
				SetLastHeartbeat(t)
			case models := <-modelsCh:
				msg := ctrl.ModelsUpdateMessage{Type: "models_update", Models: models}
				bb, _ := json.Marshal(msg)
				sendCh <- bb
			}
		}
	}()

	checkDrain := func() {
		if IsDraining() && GetState().CurrentJobs == 0 {
			SetState("terminating")
			go func() { _ = ws.Close(websocket.StatusNormalClosure, "drained") }()
			cancel()
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
				sendCh <- b
				continue
			}
			if jr.Endpoint == "generate" {
				go handleGenerate(connCtx, client, sendCh, jr, reqCancels, &jobMu, checkDrain)
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
				sendCh <- hb
				end := ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: hr.RequestID, Error: &ctrl.HTTPProxyError{Code: "worker_draining", Message: "worker is draining"}}
				eb, _ := json.Marshal(end)
				sendCh <- eb
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

func reconnectDelay(attempt int) time.Duration {
	schedule := []time.Duration{time.Second, time.Second, time.Second, 5 * time.Second, 5 * time.Second, 5 * time.Second, 15 * time.Second, 15 * time.Second, 15 * time.Second}
	if attempt < len(schedule) {
		return schedule[attempt]
	}
	return 30 * time.Second
}

type healthClient interface {
	Health(context.Context) ([]string, error)
}

func startHealthProbe(ctx context.Context, client healthClient, interval time.Duration, ch chan []string) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				probeOllama(ctx, client, ch)
			}
		}
	}()
}

func probeOllama(ctx context.Context, client healthClient, ch chan []string) {
	models, err := client.Health(ctx)
	if err != nil {
		SetConnectedToOllama(false)
		SetLastError(err.Error())
		return
	}
	SetConnectedToOllama(true)
	if !reflect.DeepEqual(models, GetState().Models) {
		SetModels(models)
		if ch != nil {
			select {
			case ch <- models:
			default:
				<-ch
				ch <- models
			}
		}
	}
	SetLastError("")
}

func handleGenerate(ctx context.Context, client *ollama.Client, sendCh chan []byte, jr ctrl.JobRequestMessage, cancels map[string]context.CancelFunc, mu *sync.Mutex, onDone func()) {
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
			sendCh <- b
			SetLastError(err.Error())
			return
		}
		defer func() {
			_ = rc.Close()
		}()
		for line := range ollama.ReadLines(rc) {
			msg := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(line)}
			b, _ := json.Marshal(msg)
			sendCh <- b
		}
		done := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(`{"done":true}`)}
		b, _ := json.Marshal(done)
		sendCh <- b
	} else {
		data, err := client.Generate(jobCtx, req)
		if err != nil {
			logx.Log.Error().Str("job", jr.JobID).Err(err).Msg("generate error")
			msg := ctrl.JobErrorMessage{Type: "job_error", JobID: jr.JobID, Code: "error", Message: err.Error()}
			b, _ := json.Marshal(msg)
			sendCh <- b
			SetLastError(err.Error())
			return
		}
		msg := ctrl.JobResultMessage{Type: "job_result", JobID: jr.JobID, Data: json.RawMessage(data)}
		b, _ := json.Marshal(msg)
		sendCh <- b
		success = true
	}
	if req.Stream {
		success = true
	}
}
