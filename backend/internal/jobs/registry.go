// Package jobs tracks the heartbeat/last-run status of background schedulers
// (certificate renewal, approval expiry, host monitoring) for operational
// visibility in the admin UI.
package jobs

import (
	"sync"
	"time"
)

// Status is a snapshot of one background scheduler.
type Status struct {
	Name      string     `json:"name"`
	Runs      int64      `json:"runs"`
	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	LastError string     `json:"lastError,omitempty"`
	OK        bool       `json:"ok"`
}

// Registry holds scheduler statuses. It is safe for concurrent use.
type Registry struct {
	mu    sync.Mutex
	items map[string]*Status
	order []string
	now   func() time.Time
}

// NewRegistry constructs an empty Registry.
func NewRegistry() *Registry {
	return &Registry{items: map[string]*Status{}, now: time.Now}
}

// Record notes a scheduler run and its outcome (err nil means success).
func (r *Registry) Record(name string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.items[name]
	if !ok {
		s = &Status{Name: name}
		r.items[name] = s
		r.order = append(r.order, name)
	}
	s.Runs++
	t := r.now()
	s.LastRunAt = &t
	if err != nil {
		s.LastError = err.Error()
		s.OK = false
	} else {
		s.LastError = ""
		s.OK = true
	}
}

// Snapshot returns the current statuses in registration order.
func (r *Registry) Snapshot() []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Status, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, *r.items[name])
	}
	return out
}
