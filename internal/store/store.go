// Package store keeps the in-memory check history, derives per-service
// status and uptime statistics, and records incidents (status transitions).
//
// A Store is safe for concurrent use. History is bounded per service (see
// Config.History); everything lives in memory and is lost on restart by
// design.
package store

import (
	"sync"
	"time"

	"simple-status/internal/config"
)

// Status of a service as derived from its check history.
type Status string

const (
	// StatusPending means no result has been recorded yet.
	StatusPending Status = "pending"
	// StatusUp means the last check (or the checks before it, per retries) succeeded.
	StatusUp Status = "up"
	// StatusDown means at least `retries` consecutive checks failed.
	StatusDown Status = "down"
)

// maxIncidents bounds the incident log kept per service.
const maxIncidents = 100

// Entry is a single recorded check result.
type Entry struct {
	Timestamp time.Time
	Duration  time.Duration
	Success   bool
	Message   string
}

// Incident is a period during which a service was down. An open incident
// (ongoing outage) has a nil EndedAt.
type Incident struct {
	StartedAt time.Time
	EndedAt   *time.Time
}

// ServiceSnapshot is a consistent, detached view of a service's state,
// produced for the API and the status page.
type ServiceSnapshot struct {
	ID          string
	Name        string
	Group       string
	Type        string
	Status      Status
	LastCheck   time.Time
	LastLatency time.Duration
	LastError   string
	DownSince   *time.Time
	// Uptime24h/7d/30d are the share of successful checks in the given
	// window; nil means "no data in this window".
	Uptime24h *float64
	Uptime7d  *float64
	Uptime30d *float64
	// History holds the stored entries, oldest first.
	History []Entry
	// Incidents holds recent down periods, oldest first.
	Incidents []Incident
}

// Summary is the aggregate view of all services.
type Summary struct {
	Overall  Status
	Total    int
	Up       int
	Down     int
	Pending  int
	Services []ServiceSnapshot
}

// serviceState is the mutable per-service data guarded by Store.mu.
type serviceState struct {
	meta        config.Service
	hist        *ring
	status      Status
	fails       int
	lastCheck   time.Time
	lastLatency time.Duration
	lastError   string
	downSince   *time.Time
	incidents   []Incident
}

// Store holds the state of all configured services.
type Store struct {
	mu       sync.RWMutex
	order    []string
	services map[string]*serviceState
}

// New creates a Store for the services in cfg, preserving their order.
func New(cfg *config.Config) *Store {
	s := &Store{services: make(map[string]*serviceState, len(cfg.Services))}
	for _, svc := range cfg.Services {
		s.order = append(s.order, svc.ID)
		s.services[svc.ID] = &serviceState{
			meta:   svc,
			hist:   newRing(cfg.History),
			status: StatusPending,
		}
	}
	return s
}

// Record applies a check result and updates the derived status. It returns
// the previous and the new status so callers can log transitions.
func (s *Store) Record(id string, e Entry) (prev, curr Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.services[id]
	if st == nil {
		return StatusPending, StatusPending
	}
	prev = st.status

	if e.Success {
		st.fails = 0
		st.lastError = ""
		if st.status != StatusUp {
			st.closeOpenIncident(e.Timestamp)
			st.status = StatusUp
			st.downSince = nil
		}
	} else {
		st.fails++
		st.lastError = e.Message
		if st.fails >= st.meta.Retries && st.status != StatusDown {
			started := e.Timestamp
			st.status = StatusDown
			st.downSince = &started
			st.incidents = append(st.incidents, Incident{StartedAt: started})
			if len(st.incidents) > maxIncidents {
				st.incidents = st.incidents[len(st.incidents)-maxIncidents:]
			}
		}
	}

	st.lastCheck = e.Timestamp
	st.lastLatency = e.Duration
	st.hist.push(e)
	return prev, st.status
}

// closeOpenIncident stamps the end time onto the most recent incident.
func (st *serviceState) closeOpenIncident(at time.Time) {
	if n := len(st.incidents); n > 0 && st.incidents[n-1].EndedAt == nil {
		ended := at
		st.incidents[n-1].EndedAt = &ended
	}
}

// Summary returns a snapshot of all services in configuration order.
func (s *Store) Summary(now time.Time) Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sum := Summary{Total: len(s.order), Services: make([]ServiceSnapshot, 0, len(s.order))}
	for _, id := range s.order {
		snap := s.services[id].snapshot(now)
		sum.Services = append(sum.Services, snap)
		switch snap.Status {
		case StatusUp:
			sum.Up++
		case StatusDown:
			sum.Down++
		default:
			sum.Pending++
		}
	}
	sum.Overall = overallStatus(sum.Up, sum.Down, sum.Pending)
	return sum
}

// Service returns a snapshot of a single service; ok is false for unknown ids.
func (s *Store) Service(id string, now time.Time) (ServiceSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st, ok := s.services[id]
	if !ok {
		return ServiceSnapshot{}, false
	}
	return st.snapshot(now), true
}

func overallStatus(up, down, pending int) Status {
	switch {
	case down > 0:
		return StatusDown
	case pending > 0 && up == 0:
		return StatusPending
	default:
		return StatusUp
	}
}

// snapshot builds a detached view of the service state; callers must hold
// at least a read lock.
func (st *serviceState) snapshot(now time.Time) ServiceSnapshot {
	snap := ServiceSnapshot{
		ID:          st.meta.ID,
		Name:        st.meta.Name,
		Group:       st.meta.Group,
		Type:        st.meta.Type,
		Status:      st.status,
		LastCheck:   st.lastCheck,
		LastLatency: st.lastLatency,
		LastError:   st.lastError,
		DownSince:   st.downSince,
		History:     st.hist.slice(),
	}
	if st.downSince != nil {
		since := *st.downSince
		snap.DownSince = &since
	}
	snap.Uptime24h = st.hist.uptime(24*time.Hour, now)
	snap.Uptime7d = st.hist.uptime(7*24*time.Hour, now)
	snap.Uptime30d = st.hist.uptime(30*24*time.Hour, now)
	snap.Incidents = append([]Incident(nil), st.incidents...)
	return snap
}
