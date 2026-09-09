package store

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/MCTzOCK/simple-status/internal/config"
)

func testConfig(retries int, history int) *config.Config {
	if history == 0 {
		history = 10
	}
	return &config.Config{
		History: history,
		Services: []config.Service{
			{ID: "web", Name: "Website", Type: "http", Retries: retries},
		},
	}
}

func entryAt(offset time.Duration, success bool) Entry {
	return Entry{
		Timestamp: time.Now().Add(offset),
		Duration:  100 * time.Millisecond,
		Success:   success,
		Message:   "msg",
	}
}

func statusOf(t *testing.T, s *Store, id string) Status {
	t.Helper()
	snap, ok := s.Service(id, time.Now())
	if !ok {
		t.Fatal("service not found")
	}
	return snap.Status
}

func TestStatusLifecycle(t *testing.T) {
	s := New(testConfig(2, 0))

	if got := statusOf(t, s, "web"); got != StatusPending {
		t.Fatalf("initial status = %s, want pending", got)
	}

	// First success flips pending -> up and records no incident.
	if prev, curr := s.Record("web", entryAt(-3*time.Minute, true)); prev != StatusPending || curr != StatusUp {
		t.Fatalf("pending->up, got %s->%s", prev, curr)
	}

	// With retries=2 a single failure keeps the service up.
	s.Record("web", entryAt(-2*time.Minute, false))
	if got := statusOf(t, s, "web"); got != StatusUp {
		t.Fatalf("after one failure = %s, want up (retries=2)", got)
	}

	// The second consecutive failure flips it down and opens an incident.
	if prev, curr := s.Record("web", entryAt(-time.Minute, false)); prev != StatusUp || curr != StatusDown {
		t.Fatalf("up->down, got %s->%s", prev, curr)
	}
	snap, _ := s.Service("web", time.Now())
	if len(snap.Incidents) != 1 || snap.Incidents[0].EndedAt != nil {
		t.Fatalf("want one open incident, got %+v", snap.Incidents)
	}
	if snap.DownSince == nil {
		t.Fatal("DownSince not set while down")
	}
	if snap.LastError == "" {
		t.Fatal("LastError not recorded")
	}

	// Any success recovers immediately and closes the incident.
	s.Record("web", entryAt(0, true))
	snap, _ = s.Service("web", time.Now())
	if snap.Status != StatusUp {
		t.Fatalf("after recovery = %s, want up", snap.Status)
	}
	if len(snap.Incidents) != 1 || snap.Incidents[0].EndedAt == nil {
		t.Fatalf("incident not closed: %+v", snap.Incidents)
	}
	if snap.LastError != "" {
		t.Fatalf("LastError not cleared: %q", snap.LastError)
	}
}

func TestRingEviction(t *testing.T) {
	s := New(testConfig(1, 5))
	// Push entries in chronological order (oldest first), 7m ago → now.
	for i := 7; i >= 0; i-- {
		s.Record("web", entryAt(-time.Duration(i)*time.Minute, true))
	}
	snap, _ := s.Service("web", time.Now())
	if len(snap.History) != 5 {
		t.Fatalf("history length = %d, want capacity 5", len(snap.History))
	}
	// Survivors are the 5 most recent: ages 4m..0m.
	if got := time.Since(snap.History[0].Timestamp); got < 3*time.Minute || got > 5*time.Minute {
		t.Fatalf("oldest surviving entry age = %v, want ~4m", got)
	}
	if got := time.Since(snap.History[len(snap.History)-1].Timestamp); got > time.Minute {
		t.Fatalf("newest surviving entry age = %v, want ~0m", got)
	}
}

func TestUptimeWindows(t *testing.T) {
	s := New(testConfig(1, 100))
	now := time.Now()

	// 3 successes inside 24h, 1 failure 3 days ago (outside 24h, inside 7d).
	s.Record("web", Entry{Timestamp: now.Add(-1 * time.Hour), Success: true})
	s.Record("web", Entry{Timestamp: now.Add(-2 * time.Hour), Success: true})
	s.Record("web", Entry{Timestamp: now.Add(-3 * time.Hour), Success: false})
	s.Record("web", Entry{Timestamp: now.Add(-72 * time.Hour), Success: false})

	snap, _ := s.Service("web", now)
	if got := *snap.Uptime24h; abs(got-100*(2.0/3.0)) > 0.001 {
		t.Fatalf("Uptime24h = %v, want %v", got, 100*(2.0/3.0))
	}
	if got := *snap.Uptime7d; got != 50 {
		t.Fatalf("Uptime7d = %v, want 50", got)
	}
	if got := *snap.Uptime30d; got != 50 {
		t.Fatalf("Uptime30d = %v, want 50", got)
	}

	// A store with no results at all reports nil (no data).
	empty := New(testConfig(1, 10))
	snap, _ = empty.Service("web", now)
	if snap.Uptime24h != nil || snap.Uptime7d != nil || snap.Uptime30d != nil {
		t.Fatalf("want nil uptimes without data, got %v/%v/%v", snap.Uptime24h, snap.Uptime7d, snap.Uptime30d)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestSummaryCountsAndOrder(t *testing.T) {
	cfg := &config.Config{
		History: 10,
		Services: []config.Service{
			{ID: "a", Name: "A", Type: "http", Retries: 1},
			{ID: "b", Name: "B", Type: "tcp", Retries: 1},
		},
	}
	s := New(cfg)
	s.Record("b", entryAt(0, false))
	s.Record("a", entryAt(0, true))

	sum := s.Summary(time.Now())
	if sum.Overall != StatusDown || sum.Down != 1 || sum.Up != 1 || sum.Pending != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	if sum.Services[0].ID != "a" || sum.Services[1].ID != "b" {
		t.Fatalf("config order not preserved: %s, %s", sum.Services[0].ID, sum.Services[1].ID)
	}

	allPending := New(cfg).Summary(time.Now())
	if allPending.Overall != StatusPending {
		t.Fatalf("all pending overall = %s", allPending.Overall)
	}
}

func TestUnknownService(t *testing.T) {
	s := New(testConfig(1, 0))
	if _, ok := s.Service("nope", time.Now()); ok {
		t.Fatal("want ok=false for unknown service")
	}
	// Recording an unknown service is a no-op, not a panic.
	s.Record("nope", entryAt(0, true))
}

func TestConcurrentAccess(t *testing.T) {
	s := New(testConfig(1, 100))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s.Record("web", Entry{
					Timestamp: time.Now(),
					Success:   (i+j)%2 == 0,
					Message:   fmt.Sprintf("r%d", j),
				})
			}
		}(i)
	}
	// Readers run concurrently with writers.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = s.Summary(time.Now())
			}
		}()
	}
	wg.Wait()
}
