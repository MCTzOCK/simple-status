// Package api exposes the read-only JSON API and the embedded status page.
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/MCTzOCK/simple-status/internal/config"
	"github.com/MCTzOCK/simple-status/internal/store"
	"github.com/MCTzOCK/simple-status/web"
)

// summaryHistoryLen limits how many recent history entries are included per
// service in the summary payload; the service detail endpoint returns all.
const summaryHistoryLen = 60

// New returns the HTTP handler serving the status page, static assets and
// the versioned JSON API.
func New(cfg *config.Config, st *store.Store) http.Handler {
	h := &handler{title: cfg.Title, store: st}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.serveIndex)
	mux.HandleFunc("GET /healthz", h.serveHealth)
	mux.HandleFunc("GET /api/v1/summary", h.serveSummary)
	mux.HandleFunc("GET /api/v1/services/{id}", h.serveService)
	mux.Handle("GET /static/", http.StripPrefix("/static", http.FileServerFS(web.Files)))
	return mux
}

type handler struct {
	title string
	store *store.Store
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	index, err := web.Files.ReadFile("index.html")
	if err != nil {
		http.Error(w, "index.html missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(index); err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)
	}
}

func (h *handler) serveHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("ok\n"))
}

func (h *handler) serveSummary(w http.ResponseWriter, r *http.Request) {
	sum := h.store.Summary(time.Now())
	writeJSON(w, http.StatusOK, summaryJSON{
		Title:     h.title,
		UpdatedAt: time.Now(),
		Overall:   string(sum.Overall),
		Counts: countsJSON{
			Total:   sum.Total,
			Up:      sum.Up,
			Down:    sum.Down,
			Pending: sum.Pending,
		},
		Services: toServiceJSONs(sum.Services, summaryHistoryLen, false),
	})
}

func (h *handler) serveService(w http.ResponseWriter, r *http.Request) {
	snap, ok := h.store.Service(r.PathValue("id"), time.Now())
	if !ok {
		writeJSON(w, http.StatusNotFound, errorJSON{Error: "service not found"})
		return
	}
	writeJSON(w, http.StatusOK, detailJSON{Service: toServiceJSON(snap, 0, true)})
}

// --- JSON DTOs ---

type summaryJSON struct {
	Title     string        `json:"title"`
	UpdatedAt time.Time     `json:"updated_at"`
	Overall   string        `json:"overall"`
	Counts    countsJSON    `json:"counts"`
	Services  []serviceJSON `json:"services"`
}

type countsJSON struct {
	Total   int `json:"total"`
	Up      int `json:"up"`
	Down    int `json:"down"`
	Pending int `json:"pending"`
}

type detailJSON struct {
	Service serviceJSON `json:"service"`
}

type serviceJSON struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Group         string         `json:"group,omitempty"`
	Type          string         `json:"type"`
	Status        string         `json:"status"`
	LastCheck     time.Time      `json:"last_check"`
	LastLatencyMS float64        `json:"last_latency_ms"`
	LastError     string         `json:"last_error,omitempty"`
	DownSince     *time.Time     `json:"down_since,omitempty"`
	Uptime        uptimeJSON     `json:"uptime"`
	History       []historyEntry `json:"history"`
	Incidents     []incidentJSON `json:"incidents,omitempty"`
}

type uptimeJSON struct {
	H24 *float64 `json:"24h"`
	D7  *float64 `json:"7d"`
	D30 *float64 `json:"30d"`
}

type historyEntry struct {
	At         time.Time `json:"at"`
	DurationMS float64   `json:"duration_ms"`
	OK         bool      `json:"ok"`
}

type incidentJSON struct {
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
}

// toServiceJSONs maps snapshots to DTOs; limit caps the history length
// (0 = all) and includeIncidents controls the incident log.
func toServiceJSONs(snaps []store.ServiceSnapshot, limit int, includeIncidents bool) []serviceJSON {
	out := make([]serviceJSON, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, toServiceJSON(s, limit, includeIncidents))
	}
	return out
}

func toServiceJSON(s store.ServiceSnapshot, limit int, includeIncidents bool) serviceJSON {
	history := make([]historyEntry, 0, len(s.History))
	for _, e := range s.History {
		history = append(history, historyEntry{
			At:         e.Timestamp,
			DurationMS: float64(e.Duration) / float64(time.Millisecond),
			OK:         e.Success,
		})
	}
	if limit > 0 && len(history) > limit {
		history = history[len(history)-limit:]
	}

	out := serviceJSON{
		ID:            s.ID,
		Name:          s.Name,
		Group:         s.Group,
		Type:          s.Type,
		Status:        string(s.Status),
		LastCheck:     s.LastCheck,
		LastLatencyMS: float64(s.LastLatency) / float64(time.Millisecond),
		LastError:     s.LastError,
		DownSince:     s.DownSince,
		Uptime: uptimeJSON{
			H24: s.Uptime24h,
			D7:  s.Uptime7d,
			D30: s.Uptime30d,
		},
		History: history,
	}
	if includeIncidents {
		for _, inc := range s.Incidents {
			out.Incidents = append(out.Incidents, incidentJSON{
				StartedAt: inc.StartedAt,
				EndedAt:   inc.EndedAt,
			})
		}
	}
	return out
}

type errorJSON struct {
	Error string `json:"error"`
}

// writeJSON renders v as a JSON response with no-store caching.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// After WriteHeader an encoding error can no longer be surfaced to the
	// client, so it is deliberately ignored.
	_ = json.NewEncoder(w).Encode(v)
}
