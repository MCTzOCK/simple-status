package prober

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/MCTzOCK/simple-status/internal/config"
)

func init() { Register("tcp", newTCPProbe) }

// tcpProbe verifies that a TCP connection can be established to host:port.
type tcpProbe struct {
	target string
}

func newTCPProbe(svc config.Service) (Probe, error) {
	host, port, err := net.SplitHostPort(svc.Target)
	if err != nil {
		return nil, fmt.Errorf("tcp target %q must be host:port: %w", svc.Target, err)
	}
	if _, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("tcp target %q has non-numeric port %q", svc.Target, port)
	}
	if host == "" {
		return nil, fmt.Errorf("tcp target %q is missing a host", svc.Target)
	}
	return &tcpProbe{target: svc.Target}, nil
}

func (p *tcpProbe) Check(ctx context.Context) Result {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", p.target)
	if err != nil {
		return Result{Success: false, Message: err.Error()}
	}
	if err := conn.Close(); err != nil {
		return Result{Success: false, Message: fmt.Sprintf("close connection: %v", err)}
	}
	return Result{Success: true, Message: "connection established"}
}
