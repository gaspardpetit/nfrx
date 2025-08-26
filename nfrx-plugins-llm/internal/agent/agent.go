package agent

import (
	"context"
	"time"

	ctrlpb "github.com/gaspardpetit/nfrx-sdk/ctrl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the control plane gRPC client.
type Client struct {
	conn *grpc.ClientConn
	cli  ctrlpb.ControlClient
}

// Dial connects to the control plane gRPC server.
func Dial(ctx context.Context, addr string) (*Client, error) {
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, cli: ctrlpb.NewControlClient(conn)}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error { return c.conn.Close() }

// Register registers the worker with the control plane.
func (c *Client) Register(ctx context.Context, req *ctrlpb.RegisterRequest, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	_, err := c.cli.Register(ctx, req)
	return err
}

// StartHeartbeat launches a heartbeat loop.
func (c *Client) StartHeartbeat(ctx context.Context, workerID string, interval time.Duration) error {
	stream, err := c.cli.Heartbeat(ctx)
	if err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = stream.CloseSend()
				return
			case <-ticker.C:
				_ = stream.Send(&ctrlpb.ControlHeartbeat{WorkerId: workerID, Load: 0})
			}
		}
	}()
	return nil
}
