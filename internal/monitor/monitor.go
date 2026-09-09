// Package monitor runs one goroutine per configured service, probing it on
// its interval and recording results in the store.
package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/MCTzOCK/simple-status/internal/config"
	"github.com/MCTzOCK/simple-status/internal/prober"
	"github.com/MCTzOCK/simple-status/internal/store"
)

// Monitor owns the check loops for all services.
type Monitor struct {
	store  *store.Store
	log    *slog.Logger
	probes []serviceProbe
}

// serviceProbe pairs a configured service with its built probe.
type serviceProbe struct {
	svc   config.Service
	probe prober.Probe
}

// New builds a probe for every configured service. It returns an error if
// any service has an invalid target so that misconfiguration fails at
// startup rather than during the first check.
func New(cfg *config.Config, st *store.Store, log *slog.Logger) (*Monitor, error) {
	m := &Monitor{store: st, log: log}
	for _, svc := range cfg.Services {
		probe, err := prober.New(svc)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", svc.ID, err)
		}
		m.probes = append(m.probes, serviceProbe{svc: svc, probe: probe})
	}
	return m, nil
}

// Run starts one check loop per service and blocks until ctx is cancelled
// (all in-flight checks are given their per-service timeout to finish).
func (m *Monitor) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, sp := range m.probes {
		wg.Add(1)
		go func(sp serviceProbe) {
			defer wg.Done()
			m.loop(ctx, sp)
		}(sp)
	}
	wg.Wait()
}

func (m *Monitor) loop(ctx context.Context, sp serviceProbe) {
	m.check(ctx, sp) // probe immediately on startup instead of waiting a full interval
	ticker := time.NewTicker(sp.svc.Interval.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check(ctx, sp)
		}
	}
}

// check runs a single probe with the service's timeout and records the
// result, logging status transitions.
func (m *Monitor) check(parent context.Context, sp serviceProbe) {
	ctx, cancel := context.WithTimeout(parent, sp.svc.Timeout.Duration)
	defer cancel()

	start := time.Now()
	res := sp.probe.Check(ctx)
	res.Duration = time.Since(start)

	prev, curr := m.store.Record(sp.svc.ID, store.Entry{
		Timestamp: start,
		Duration:  res.Duration,
		Success:   res.Success,
		Message:   res.Message,
	})
	if prev != curr {
		switch {
		case curr == store.StatusDown:
			m.log.Warn("service is down", "service", sp.svc.ID, "error", res.Message)
		case prev == store.StatusPending:
			m.log.Info("service is up", "service", sp.svc.ID)
		default:
			m.log.Info("service recovered", "service", sp.svc.ID)
		}
	}
}

// CheckOnce probes every service a single time, in parallel, and waits for
// all results. It is used by the --once mode.
func (m *Monitor) CheckOnce(ctx context.Context) {
	var wg sync.WaitGroup
	for _, sp := range m.probes {
		wg.Add(1)
		go func(sp serviceProbe) {
			defer wg.Done()
			m.check(ctx, sp)
		}(sp)
	}
	wg.Wait()
}
