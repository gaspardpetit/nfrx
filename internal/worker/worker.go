package worker

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/logx"
	"github.com/you/llamapool/internal/ollama"
	"github.com/you/llamapool/internal/relay"
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

	go startHealthProbe(ctx, client, 20*time.Second)

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

	ws, _, err := websocket.Dial(ctx, cfg.ServerURL, nil)
	if err != nil {
		SetLastError(err.Error())
		return err
	}
	defer func() {
		_ = ws.Close(websocket.StatusInternalError, "closing")
	}()

	SetConnectedToServer(true)
	SetState("connected_idle")
	SetLastError("")

	sendCh := make(chan []byte, 16)
	reqCancels := make(map[string]context.CancelFunc)
	var jobMu sync.Mutex
	go func() {
		for msg := range sendCh {
			if err := ws.Write(ctx, websocket.MessageText, msg); err != nil {
				return
			}
		}
	}()

	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: cfg.WorkerID, WorkerName: cfg.WorkerName, WorkerKey: cfg.WorkerKey, Models: models, MaxConcurrency: cfg.MaxConcurrency}
	b, _ := json.Marshal(regMsg)
	sendCh <- b

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				hb := ctrl.HeartbeatMessage{Type: "heartbeat", TS: t.Unix()}
				bb, _ := json.Marshal(hb)
				sendCh <- bb
				SetLastHeartbeat(t)
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

	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			SetConnectedToServer(false)
			if IsDraining() {
				return nil
			}
			SetLastError(err.Error())
			SetState("error")
			return err
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
				msg := ctrl.JobErrorMessage{Type: "job_error", JobID: jr.JobID, Code: "worker_draining", Message: "worker is draining"}
				b, _ := json.Marshal(msg)
				sendCh <- b
				continue
			}
			if jr.Endpoint == "generate" {
				go handleGenerate(ctx, client, sendCh, jr, reqCancels, &jobMu, checkDrain)
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
				h := ctrl.HTTPProxyResponseHeadersMessage{Type: "http_proxy_response_headers", RequestID: hr.RequestID, Status: 503, Headers: map[string]string{"Content-Type": "application/json"}}
				hb, _ := json.Marshal(h)
				sendCh <- hb
				end := ctrl.HTTPProxyResponseEndMessage{Type: "http_proxy_response_end", RequestID: hr.RequestID, Error: &ctrl.HTTPProxyError{Code: "worker_draining", Message: "worker is draining"}}
				eb, _ := json.Marshal(end)
				sendCh <- eb
				continue
			}
			go handleHTTPProxy(ctx, cfg, sendCh, hr, reqCancels, &jobMu, checkDrain)
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

func startHealthProbe(ctx context.Context, client healthClient, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				probeOllama(ctx, client)
			}
		}
	}()
}

func probeOllama(ctx context.Context, client healthClient) {
	models, err := client.Health(ctx)
	if err != nil {
		SetConnectedToOllama(false)
		SetLastError(err.Error())
		return
	}
	SetConnectedToOllama(true)
	if !reflect.DeepEqual(models, GetState().Models) {
		SetModels(models)
	}
	SetLastError("")
}

func handleGenerate(ctx context.Context, client *ollama.Client, sendCh chan []byte, jr ctrl.JobRequestMessage, cancels map[string]context.CancelFunc, mu *sync.Mutex, onDone func()) {
	raw, _ := json.Marshal(jr.Payload)
	var req relay.GenerateRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		logx.Log.Error().Err(err).Msg("unmarshal generate request")
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
		JobCompleted(success, time.Since(start))
	}()
	if req.Stream {
		rc, err := client.GenerateStream(jobCtx, req)
		if err != nil {
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
