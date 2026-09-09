// Package prober defines the probe abstraction used to check services and
// provides built-in implementations (http, tcp, dns). Additional probe types
// plug in by registering a Factory.
package prober

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/MCTzOCK/simple-status/internal/config"
)

// Result is the outcome of a single check. Timestamp and Duration are filled
// in by the monitor; probes report only success, a short human-readable
// message and the measured duration.
type Result struct {
	Success  bool
	Message  string
	Duration time.Duration
}

// Probe performs a single check of a service target. Implementations must
// honor ctx (used as the check deadline) and must be safe for concurrent use.
type Probe interface {
	Check(ctx context.Context) Result
}

// Factory builds a Probe for a configured service. It validates
// type-specific options and returns an error for unusable targets so that
// misconfiguration surfaces at startup, not on the first check.
type Factory func(svc config.Service) (Probe, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register adds a factory for a probe type; intended for init functions.
// Registering the same type twice panics, as it indicates a programming error.
func Register(kind string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[kind]; dup {
		panic("prober: duplicate registration for type " + kind)
	}
	registry[kind] = f
}

// New builds a probe for the given service.
func New(svc config.Service) (Probe, error) {
	registryMu.RLock()
	f, ok := registry[svc.Type]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown probe type %q", svc.Type)
	}
	return f(svc)
}

// Types returns the registered probe type names in sorted order.
func Types() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	kinds := make([]string, 0, len(registry))
	for kind := range registry {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}
