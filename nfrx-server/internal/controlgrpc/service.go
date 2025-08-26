package controlgrpc

import (
	"context"

	ctrlpb "github.com/gaspardpetit/nfrx-sdk/ctrl"
	"github.com/gaspardpetit/nfrx-sdk/logx"
	ctrlsrv "github.com/gaspardpetit/nfrx-server/internal/ctrlsrv"
)

// Server implements the control gRPC service.
type Server struct {
	ctrlpb.UnimplementedControlServer
	reg *ctrlsrv.Registry
}

// New returns a new Server.
func New(reg *ctrlsrv.Registry) *Server {
	return &Server{reg: reg}
}

// Register handles worker registration.
func (s *Server) Register(ctx context.Context, req *ctrlpb.RegisterRequest) (*ctrlpb.RegisterResponse, error) {
	logx.Log.Info().Str("worker_id", req.GetWorkerId()).Msg("register via grpc")
	// For now just acknowledge the worker. Registry integration will follow.
	return &ctrlpb.RegisterResponse{AssignedId: req.GetWorkerId(), Accepted: true}, nil
}

// Heartbeat processes incoming heartbeats.
func (s *Server) Heartbeat(stream ctrlpb.Control_HeartbeatServer) error {
	for {
		hb, err := stream.Recv()
		if err != nil {
			return err
		}
		if hb.GetWorkerId() != "" {
			s.reg.UpdateHeartbeat(hb.GetWorkerId())
		}
		if err := stream.Send(&ctrlpb.ControlHeartbeat{WorkerId: hb.GetWorkerId()}); err != nil {
			return err
		}
	}
}
