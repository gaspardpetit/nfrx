package controlgrpc

import (
	"context"
	"strconv"
	"strings"
	"time"

	ctrlpb "github.com/gaspardpetit/nfrx-sdk/ctrl"
	"github.com/gaspardpetit/nfrx-sdk/logx"
	ctrlsrv "github.com/gaspardpetit/nfrx-server/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx-server/internal/serverstate"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the control gRPC service.
type Server struct {
	ctrlpb.UnimplementedControlServer
	reg       *ctrlsrv.Registry
	metrics   *ctrlsrv.MetricsRegistry
	clientKey string
}

// New returns a new Server.
func New(reg *ctrlsrv.Registry, metrics *ctrlsrv.MetricsRegistry, clientKey string) *Server {
	return &Server{reg: reg, metrics: metrics, clientKey: clientKey}
}

// Register handles worker registration.
func (s *Server) Register(ctx context.Context, req *ctrlpb.RegisterRequest) (*ctrlpb.RegisterResponse, error) {
	key := req.GetMetadata()["client_key"]
	if s.clientKey != "" && key != s.clientKey {
		return nil, status.Error(codes.PermissionDenied, "unauthorized")
	}
	name := req.GetMetadata()["worker_name"]
	if name == "" {
		if len(req.GetWorkerId()) >= 8 {
			name = req.GetWorkerId()[:8]
		} else {
			name = req.GetWorkerId()
		}
	}
	models := []string{}
	if ms := req.GetMetadata()["models"]; ms != "" {
		models = strings.Split(ms, ",")
	}
	maxConc := 0
	if v, err := strconv.Atoi(req.GetMetadata()["max_concurrency"]); err == nil {
		maxConc = v
	}
	embBatch := 0
	if v, err := strconv.Atoi(req.GetMetadata()["embedding_batch_size"]); err == nil {
		embBatch = v
	}
	wk := &ctrlsrv.Worker{
		ID:                 req.GetWorkerId(),
		Name:               name,
		Models:             map[string]bool{},
		Capabilities:       map[string]bool{},
		MaxConcurrency:     maxConc,
		EmbeddingBatchSize: embBatch,
		InFlight:           0,
		LastHeartbeat:      time.Now(),
		Send:               make(chan interface{}, 32),
		Jobs:               make(map[string]chan interface{}),
		ProtocolVersion:    req.GetProtocolVersion(),
	}
	for _, m := range models {
		wk.Models[m] = true
	}
	for _, c := range req.GetCapabilities() {
		wk.Capabilities[c] = true
	}
	s.reg.Add(wk)
	if s.metrics != nil {
		s.metrics.UpsertWorker(wk.ID, wk.Name, req.GetMetadata()["version"], req.GetMetadata()["build_sha"], req.GetMetadata()["build_date"], maxConc, embBatch, models)
		status := ctrlsrv.StatusIdle
		if maxConc == 0 {
			status = ctrlsrv.StatusNotReady
		}
		s.metrics.SetWorkerStatus(wk.ID, status)
	}
	if s.reg.WorkerCount() == 1 {
		serverstate.SetState("ready")
	}
	logx.Log.Info().Str("worker_id", req.GetWorkerId()).Str("worker_name", name).Int("model_count", len(models)).Msg("registered via grpc")
	return &ctrlpb.RegisterResponse{AssignedId: req.GetWorkerId(), Accepted: true}, nil
}

// Heartbeat processes incoming heartbeats.
func (s *Server) Heartbeat(stream ctrlpb.Control_HeartbeatServer) error {
	var workerID string
	for {
		hb, err := stream.Recv()
		if err != nil {
			if workerID != "" {
				s.reg.Remove(workerID)
				if s.metrics != nil {
					s.metrics.RemoveWorker(workerID)
				}
				if s.reg.WorkerCount() == 0 {
					serverstate.SetState("not_ready")
				}
			}
			return err
		}
		workerID = hb.GetWorkerId()
		if workerID != "" {
			s.reg.UpdateHeartbeat(workerID)
			if s.metrics != nil {
				s.metrics.RecordHeartbeat(workerID)
			}
		}
		if err := stream.Send(&ctrlpb.ControlHeartbeat{WorkerId: workerID}); err != nil {
			return err
		}
	}
}
