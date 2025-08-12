package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

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
	client := ollama.New(cfg.OllamaURL)
	models, err := client.Tags(ctx)
	if err != nil {
		return err
	}
	ws, _, err := websocket.Dial(ctx, cfg.ServerURL, nil)
	if err != nil {
		return err
	}
	defer ws.Close(websocket.StatusInternalError, "closing")

	sendCh := make(chan []byte, 16)
	go func() {
		for msg := range sendCh {
			ws.Write(ctx, websocket.MessageText, msg)
		}
	}()

	regMsg := ctrl.RegisterMessage{Type: "register", WorkerID: cfg.WorkerID, Token: cfg.Token, Models: models, MaxConcurrency: cfg.MaxConcurrency}
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
			}
		}
	}()

	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			return err
		}
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		if env.Type == "job_request" {
			var jr ctrl.JobRequestMessage
			if err := json.Unmarshal(data, &jr); err != nil {
				continue
			}
			logx.Log.Info().Str("job", jr.JobID).Msg("job request")
			if jr.Endpoint == "generate" {
				handleGenerate(ctx, client, sendCh, jr)
			}
		}
	}
}

func handleGenerate(ctx context.Context, client *ollama.Client, sendCh chan []byte, jr ctrl.JobRequestMessage) {
	raw, _ := json.Marshal(jr.Payload)
	var req relay.GenerateRequest
	json.Unmarshal(raw, &req)
	if req.Stream {
		rc, err := client.GenerateStream(ctx, req)
		if err != nil {
			msg := ctrl.JobErrorMessage{Type: "job_error", JobID: jr.JobID, Code: "error", Message: err.Error()}
			b, _ := json.Marshal(msg)
			sendCh <- b
			return
		}
		defer rc.Close()
		for line := range ollama.ReadLines(rc) {
			msg := ctrl.JobChunkMessage{Type: "job_chunk", JobID: jr.JobID, Data: json.RawMessage(line)}
			b, _ := json.Marshal(msg)
			sendCh <- b
		}
	} else {
		data, err := client.Generate(ctx, req)
		if err != nil {
			msg := ctrl.JobErrorMessage{Type: "job_error", JobID: jr.JobID, Code: "error", Message: err.Error()}
			b, _ := json.Marshal(msg)
			sendCh <- b
			return
		}
		msg := ctrl.JobResultMessage{Type: "job_result", JobID: jr.JobID, Data: json.RawMessage(data)}
		b, _ := json.Marshal(msg)
		sendCh <- b
	}
}
