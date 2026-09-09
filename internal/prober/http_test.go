package prober

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MCTzOCK/simple-status/internal/config"
)

func httpService(target string, mutate func(*config.Service)) config.Service {
	svc := config.Service{
		ID:      "test",
		Type:    "http",
		Target:  target,
		Timeout: config.Duration{Duration: 2 * time.Second},
	}
	if mutate != nil {
		mutate(&svc)
	}
	return svc
}

func TestHTTPProbeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Probe") != "" {
			w.Header().Set("X-Echo", r.Header.Get("X-Probe"))
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tests := []struct {
		name   string
		mutate func(*config.Service)
	}{
		{name: "plain 200"},
		{name: "body pattern matches", mutate: func(s *config.Service) {
			s.BodyPattern = `ell[o]`
		}},
		{name: "custom header sent", mutate: func(s *config.Service) {
			s.Headers = map[string]string{"X-Probe": "simple-status"}
		}},
		{name: "expected status list includes 200", mutate: func(s *config.Service) {
			s.ExpectedStatuses = []int{200, 204}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probe, err := newHTTPProbe(httpService(srv.URL, tc.mutate))
			if err != nil {
				t.Fatalf("newHTTPProbe: %v", err)
			}
			if res := probe.Check(context.Background()); !res.Success {
				t.Fatalf("want success, got %+v", res)
			}
		})
	}
}

func TestHTTPProbeFailures(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.Service)
		handler http.HandlerFunc
	}{
		{
			name:    "status 500",
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		},
		{
			name:    "status not in expected list",
			mutate:  func(s *config.Service) { s.ExpectedStatuses = []int{200} },
			handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusMovedPermanently) },
		},
		{
			name:    "body pattern does not match",
			mutate:  func(s *config.Service) { s.BodyPattern = `goodbye` },
			handler: func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hello")) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			probe, err := newHTTPProbe(httpService(srv.URL, tc.mutate))
			if err != nil {
				t.Fatalf("newHTTPProbe: %v", err)
			}
			if res := probe.Check(context.Background()); res.Success {
				t.Fatalf("want failure, got %+v", res)
			}
		})
	}
}

func TestHTTPProbeTimeout(t *testing.T) {
	blocked := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked
	}))
	defer srv.Close()
	defer close(blocked)

	probe, err := newHTTPProbe(httpService(srv.URL, func(s *config.Service) {
		s.Timeout = config.Duration{Duration: 50 * time.Millisecond}
	}))
	if err != nil {
		t.Fatalf("newHTTPProbe: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res := probe.Check(ctx)
	if res.Success {
		t.Fatal("want timeout failure")
	}
}

func TestHTTPProbeInsecureTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	// The test server logs the expected handshake failure when the client
	// verifies the self-signed certificate; keep the output clean.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)

	svc := httpService(srv.URL, func(s *config.Service) { s.InsecureTLS = true })
	probe, err := newHTTPProbe(svc)
	if err != nil {
		t.Fatalf("newHTTPProbe: %v", err)
	}
	if res := probe.Check(context.Background()); !res.Success {
		t.Fatalf("insecure_tls should accept self-signed certificate, got %+v", res)
	}

	svc.InsecureTLS = false
	probe, err = newHTTPProbe(svc)
	if err != nil {
		t.Fatalf("newHTTPProbe: %v", err)
	}
	if res := probe.Check(context.Background()); res.Success {
		t.Fatal("want TLS verification failure without insecure_tls")
	}
}

func TestHTTPProbeTargetValidation(t *testing.T) {
	if _, err := newHTTPProbe(httpService("example.com", nil)); err == nil {
		t.Fatal("want error for target without scheme")
	}
	if _, err := newHTTPProbe(httpService("https://x.example", func(s *config.Service) {
		s.BodyPattern = "([unclosed"
	})); err == nil {
		t.Fatal("want error for invalid body_pattern")
	}
}
