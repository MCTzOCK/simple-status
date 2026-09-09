package prober

import (
	"context"
	"testing"
	"time"

	"github.com/MCTzOCK/simple-status/internal/config"
)

func dnsService(mutate func(*config.Service)) config.Service {
	svc := config.Service{ID: "test", Type: "dns", Target: "localhost", RecordType: "A"}
	if mutate != nil {
		mutate(&svc)
	}
	return svc
}

func TestDNSProbeLocalhost(t *testing.T) {
	// "localhost" resolves from hosts files without network access.
	probe, err := newDNSProbe(dnsService(nil))
	if err != nil {
		t.Fatalf("newDNSProbe: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if res := probe.Check(ctx); !res.Success {
		t.Fatalf("want success for localhost, got %+v", res)
	}
}

func TestDNSProbeUnreachableResolver(t *testing.T) {
	// ".invalid" is reserved (RFC 6761) and never resolves from hosts files,
	// so the probe must actually query the unreachable resolver.
	probe, err := newDNSProbe(dnsService(func(s *config.Service) {
		s.Target = "test.invalid"
		s.Resolver = "127.0.0.1:1"
	}))
	if err != nil {
		t.Fatalf("newDNSProbe: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if res := probe.Check(ctx); res.Success {
		t.Fatalf("want failure for unreachable resolver, got %+v", res)
	}
}

func TestDNSProbeResolverPortDefault(t *testing.T) {
	// A bare host must be accepted and get :53 appended; an unreachable
	// resolver then proves the address was actually used.
	probe, err := newDNSProbe(dnsService(func(s *config.Service) {
		s.Target = "test.invalid"
		s.Resolver = "127.0.0.1"
	}))
	if err != nil {
		t.Fatalf("newDNSProbe: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if res := probe.Check(ctx); res.Success {
		t.Fatalf("want failure via default port 53, got %+v", res)
	}
}

func TestDNSProbeRecordValidation(t *testing.T) {
	if _, err := newDNSProbe(dnsService(func(s *config.Service) { s.RecordType = "SMTP" })); err == nil {
		t.Fatal("want error for unsupported record type")
	}
}
