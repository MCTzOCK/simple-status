package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MCTzOCK/simple-status/internal/config"
	"github.com/MCTzOCK/simple-status/internal/store"
)

func testSetup(t *testing.T) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Title:   "Test Status",
		History: 10,
		Services: []config.Service{
			{ID: "web", Name: "Website", Group: "Frontend", Type: "http", Retries: 1},
			{ID: "db", Name: "Database", Type: "tcp", Retries: 1},
		},
	}
	st := store.New(cfg)
	now := time.Now()
	st.Record("web", store.Entry{Timestamp: now.Add(-time.Minute), Duration: 150 * time.Millisecond, Success: true, Message: "200 OK"})
	st.Record("db", store.Entry{Timestamp: now.Add(-time.Minute), Duration: 10 * time.Millisecond, Success: false, Message: "connection refused"})
	st.Record("db", store.Entry{Timestamp: now.Add(-30 * time.Second), Duration: 12 * time.Millisecond, Success: false, Message: "connection refused"})
	return New(cfg, st)
}

func TestSummaryEndpoint(t *testing.T) {
	srv := httptest.NewServer(testSetup(t))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/summary")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}

	var body struct {
		Title   string `json:"title"`
		Overall string `json:"overall"`
		Counts  struct {
			Total, Up, Down, Pending int
		} `json:"counts"`
		Services []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Uptime struct {
				H24 *float64 `json:"24h"`
				D7  *float64 `json:"7d"`
				D30 *float64 `json:"30d"`
			} `json:"uptime"`
			History []struct {
				At         time.Time `json:"at"`
				DurationMS float64   `json:"duration_ms"`
				OK         bool      `json:"ok"`
			} `json:"history"`
		} `json:"services"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if body.Title != "Test Status" {
		t.Errorf("title = %q", body.Title)
	}
	if body.Overall != "down" || body.Counts.Down != 1 || body.Counts.Up != 1 {
		t.Errorf("overall/counts = %s %+v", body.Overall, body.Counts)
	}
	if len(body.Services) != 2 {
		t.Fatalf("services = %d, want 2", len(body.Services))
	}
	webSvc := body.Services[0]
	if webSvc.Status != "up" || len(webSvc.History) != 1 {
		t.Errorf("web service = %+v", webSvc)
	}
	if webSvc.History[0].DurationMS != 150 {
		t.Errorf("duration_ms = %v, want 150", webSvc.History[0].DurationMS)
	}
	if body.Services[1].Uptime.H24 == nil || *body.Services[1].Uptime.H24 != 0 {
		t.Errorf("db uptime24h = %v, want 0", body.Services[1].Uptime.H24)
	}
}

func TestServiceDetailEndpoint(t *testing.T) {
	srv := httptest.NewServer(testSetup(t))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/services/db")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var body struct {
		Service struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			LastError string `json:"last_error"`
			History   []struct {
				OK bool `json:"ok"`
			} `json:"history"`
			Incidents []struct {
				StartedAt time.Time  `json:"started_at"`
				EndedAt   *time.Time `json:"ended_at"`
			} `json:"incidents"`
		} `json:"service"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	svc := body.Service
	if svc.ID != "db" || svc.Status != "down" || svc.LastError != "connection refused" {
		t.Errorf("service = %+v", svc)
	}
	if len(svc.History) != 2 {
		t.Errorf("history = %d entries, want 2 (summary caps, detail does not)", len(svc.History))
	}
	if len(svc.Incidents) != 1 || svc.Incidents[0].EndedAt != nil {
		t.Errorf("incidents = %+v, want one open incident", svc.Incidents)
	}
}

func TestServiceNotFound(t *testing.T) {
	srv := httptest.NewServer(testSetup(t))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/services/nope")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHealthAndIndexAndStatic(t *testing.T) {
	srv := httptest.NewServer(testSetup(t))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}

	for path, contentType := range map[string]string{
		"/":                 "text/html; charset=utf-8",
		"/static/app.js":    "text/javascript; charset=utf-8",
		"/static/style.css": "text/css; charset=utf-8",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != contentType {
			t.Errorf("GET %s: Content-Type = %q, want %q", path, ct, contentType)
		}
	}
}
