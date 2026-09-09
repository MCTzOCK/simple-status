package monitor

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MCTzOCK/simple-status/internal/config"
	"github.com/MCTzOCK/simple-status/internal/prober"
	"github.com/MCTzOCK/simple-status/internal/store"
)

// fake controls all probes of type "fake"; sequence holds the results each
// successive check returns (cycling when exhausted).
type fake struct {
	checks atomic.Int64
}

func (f *fake) results() prober.Result {
	n := f.checks.Add(1) - 1
	// Even checks succeed, odd checks fail.
	if n%2 == 0 {
		return prober.Result{Success: true, Message: "fake ok"}
	}
	return prober.Result{Success: false, Message: "fake failure"}
}

var sharedFake = &fake{}

func init() {
	prober.Register("fake", func(svc config.Service) (prober.Probe, error) {
		return fakeProbe{}, nil
	})
}

type fakeProbe struct{}

func (fakeProbe) Check(ctx context.Context) prober.Result { return sharedFake.results() }

func testConfig(retries int) *config.Config {
	return &config.Config{
		Interval: config.Duration{Duration: 10 * time.Millisecond},
		Timeout:  config.Duration{Duration: time.Second},
		History:  50,
		Services: []config.Service{
			{ID: "svc", Name: "svc", Type: "fake", Target: "x",
				Interval: config.Duration{Duration: 10 * time.Millisecond},
				Timeout:  config.Duration{Duration: time.Second},
				Retries:  retries},
		},
	}
}

func newMonitor(t *testing.T, retries int) (*Monitor, *store.Store) {
	t.Helper()
	st := store.New(testConfig(retries))
	mon, err := New(testConfig(retries), st, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return mon, st
}

// waitFor polls the store until cond passes or the deadline expires.
func waitFor(t *testing.T, st *store.Store, want store.Status) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snap, _ := st.Service("svc", time.Now()); snap.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	snap, _ := st.Service("svc", time.Now())
	t.Fatalf("service never reached status %s (now %s)", want, snap.Status)
}

func TestRunAlternatesStatus(t *testing.T) {
	sharedFake.checks.Store(0)
	mon, st := newMonitor(t, 1)

	ctx, cancel := context.WithCancel(context.Background())
	go mon.Run(ctx)
	defer cancel()

	waitFor(t, st, store.StatusUp)
	waitFor(t, st, store.StatusDown)
	waitFor(t, st, store.StatusUp)
}

func TestRunRetriesPreventFlapping(t *testing.T) {
	sharedFake.checks.Store(0)
	mon, st := newMonitor(t, 5) // 5 consecutive failures needed for down

	ctx, cancel := context.WithCancel(context.Background())
	go mon.Run(ctx)
	defer cancel()

	waitFor(t, st, store.StatusUp)

	// Alternating success/failure with retries=5 must never go down.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if snap, _ := st.Service("svc", time.Now()); snap.Status == store.StatusDown {
			t.Fatal("service went down despite retries=5 and alternating results")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCheckOnce(t *testing.T) {
	sharedFake.checks.Store(0)
	mon, st := newMonitor(t, 1)

	mon.CheckOnce(context.Background())

	snap, ok := st.Service("svc", time.Now())
	if !ok {
		t.Fatal("service missing from store")
	}
	if len(snap.History) != 1 {
		t.Fatalf("history length = %d, want exactly one entry from CheckOnce", len(snap.History))
	}
	if snap.Status != store.StatusUp {
		t.Fatalf("status = %s, want up after first success", snap.Status)
	}
}

func TestNewRejectsBadTarget(t *testing.T) {
	cfg := &config.Config{
		Interval: config.Duration{Duration: time.Second},
		Timeout:  config.Duration{Duration: time.Second},
		Services: []config.Service{
			{ID: "bad", Type: "tcp", Target: "no-host-port-here"},
		},
	}
	if _, err := New(cfg, store.New(cfg), slog.Default()); err == nil {
		t.Fatal("want error for invalid tcp target")
	}
}
