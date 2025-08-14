package worker

import (
	"context"
	"encoding/json"
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

	if cfg.StatusAddr != "" {
		if _, err := StartStatusServer(ctx, cfg.StatusAddr); err != nil {
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

	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			SetLastError(err.Error())
			SetConnectedToServer(false)
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
			if jr.Endpoint == "generate" {
				go handleGenerate(ctx, client, sendCh, jr, reqCancels, &jobMu)
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
			go handleHTTPProxy(ctx, cfg, sendCh, hr, reqCancels, &jobMu)
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

func handleGenerate(ctx context.Context, client *ollama.Client, sendCh chan []byte, jr ctrl.JobRequestMessage, cancels map[string]context.CancelFunc, mu *sync.Mutex) {
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
	defer func() {
		cancel()
		mu.Lock()
		delete(cancels, jr.JobID)
		mu.Unlock()
		DecJobs()
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
	}
}
