package worker

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/coder/websocket"
    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
    aconfig "github.com/gaspardpetit/nfrx/modules/llm/agent/internal/config"
    llmcommon "github.com/gaspardpetit/nfrx/modules/llm/common"
)

// Reproduces an edge case where the server connection drops while draining
// and an in-flight job finishes afterwards. The worker should not panic when
// the deferred drain check runs after the send channel has begun shutting down.
func TestNoPanicOnDrainAfterConnDrop(t *testing.T) {
    resetState()

    // Backend that exposes one model and a generate endpoint that waits
    // until we signal release.
    started := make(chan struct{})
    release := make(chan struct{})
    ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.URL.Path {
        case "/api/tags":
            w.Header().Set("Content-Type", "application/json")
            _, _ = w.Write([]byte(`{"models":[{"name":"m1"}]}`))
        case "/api/generate":
            // Signal that the job has started, then block.
            select {
            case <-started:
            default:
                close(started)
            }
            <-release
            w.Header().Set("Content-Type", "application/json")
            _, _ = w.Write([]byte(`{"done":true}`))
        default:
            w.WriteHeader(http.StatusNotFound)
        }
    }))
    defer ollama.Close()

    // Minimal WS server that accepts a connection, reads the register message,
    // then sends a job request and later drops the connection abruptly.
    connCh := make(chan *websocket.Conn, 1)
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        c, err := websocket.Accept(w, r, nil)
        if err != nil {
            t.Fatalf("accept: %v", err)
        }
        connCh <- c
    }))
    defer srv.Close()

    wsURL := "ws://" + srv.Listener.Addr().String()

    // Start the worker.
    cfg := aconfig.WorkerConfig{ServerURL: wsURL, CompletionBaseURL: ollama.URL + "/v1", MaxConcurrency: 1, EmbeddingBatchSize: 0}
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    errCh := make(chan error, 1)
    go func() { errCh <- Run(ctx, cfg) }()

    // Get server-side websocket and read the registration message.
    srvConn := <-connCh
    ctxR, cancelR := context.WithTimeout(context.Background(), time.Second)
    defer cancelR()
    if _, _, err := srvConn.Read(ctxR); err != nil { // register
        t.Fatalf("read register: %v", err)
    }

    // Send a job request to start a long running generate.
    jr := ctrl.JobRequestMessage{Type: "job_request", JobID: "j1", Endpoint: llmcommon.EndpointGenerate, Payload: llmcommon.GenerateRequest{Model: "m1", Prompt: "hi"}}
    jb, _ := json.Marshal(jr)
    if err := srvConn.Write(context.Background(), websocket.MessageText, jb); err != nil {
        t.Fatalf("write job: %v", err)
    }

    // Wait until the backend handler observes the generate request.
    select {
    case <-started:
    case <-time.After(2 * time.Second):
        t.Fatalf("timeout waiting for job start")
    }

    // Start draining, then drop the websocket connection before the job finishes.
    StartDrain()
    _ = srvConn.Close(websocket.StatusInternalError, "boom")

    // Allow the backend to finish. The worker's job context may already be
    // canceled due to conn drop, but the worker must not panic regardless.
    close(release)

    // Worker should exit cleanly (err == nil) without panicking.
    select {
    case err := <-errCh:
        if err != nil {
            t.Fatalf("worker returned error: %v", err)
        }
    case <-time.After(5 * time.Second):
        t.Fatalf("timeout waiting for worker exit")
    }
}
