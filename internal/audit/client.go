// Package audit provides a client for the audit service.
package audit

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps a gRPC connection to the audit service.
type Client struct {
	conn   *grpc.ClientConn
	target string
}

// New creates a new audit client. Returns nil if url is empty (audit disabled).
func New(ctx context.Context, url string) (*Client, error) {
	if url == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		url,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	slog.Info("audit client connected", "target", url)
	return &Client{conn: conn, target: url}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// LogEvent sends an audit event asynchronously (fire-and-forget).
// If the client is nil or disabled, this is a no-op.
func (c *Client) LogEvent(ctx context.Context, action, resource, actor string, meta map[string]string) {
	if c == nil {
		return
	}

	go func() {
		_, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// TODO: Import generated audit proto client and call IngestEvent
		// For now, just log that we would send it
		slog.Debug("audit event (would send to gRPC)",
			"action", action,
			"resource", resource,
			"actor", actor,
			"meta", meta,
			"target", c.target,
		)
	}()
}
