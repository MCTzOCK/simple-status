package prober

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"simple-status/internal/config"
)

func tcpService(target string) config.Service {
	return config.Service{ID: "test", Type: "tcp", Target: target}
}

func TestTCPProbeSuccess(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	probe, err := newTCPProbe(tcpService(listener.Addr().String()))
	if err != nil {
		t.Fatalf("newTCPProbe: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if res := probe.Check(ctx); !res.Success {
		t.Fatalf("want success, got %+v", res)
	}
}

func TestTCPProbeRefused(t *testing.T) {
	// Reserve a port, then close the listener so nothing is listening.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	probe, err := newTCPProbe(tcpService(addr))
	if err != nil {
		t.Fatalf("newTCPProbe: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if res := probe.Check(ctx); res.Success {
		t.Fatalf("want failure, got %+v", res)
	}
}

func TestTCPProbeTargetValidation(t *testing.T) {
	tests := []struct{ target, wantErr string }{
		{"noport", "must be host:port"},
		{"host:notaport", "non-numeric port"},
		{":8080", "missing a host"},
	}
	for _, tc := range tests {
		if _, err := newTCPProbe(tcpService(tc.target)); err == nil {
			t.Errorf("target %q: want error", tc.target)
		} else if !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("target %q: error %q does not contain %q", tc.target, err, tc.wantErr)
		}
	}
}
