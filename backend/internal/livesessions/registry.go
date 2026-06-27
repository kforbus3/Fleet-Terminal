// Package livesessions tracks in-flight terminal/SFTP connections so they can be
// forcibly closed when their browser session is revoked (logout, idle/absolute
// timeout, account disable/delete, or admin terminate). An established SSH
// connection is not re-checked by sshd mid-session, so cutting access requires
// actively closing the connection — this registry makes that possible.
package livesessions

import (
	"sync"

	"github.com/google/uuid"
)

// Registry maps a browser session id to the close functions of its live
// connections. Safe for concurrent use.
type Registry struct {
	mu    sync.Mutex
	conns map[uuid.UUID]map[uint64]func()
	next  uint64
}

// New constructs an empty Registry.
func New() *Registry { return &Registry{conns: make(map[uuid.UUID]map[uint64]func())} }

// Register records a live connection for a session and returns a deregister func
// the caller must invoke when the connection ends normally.
func (r *Registry) Register(sessionID uuid.UUID, closer func()) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	id := r.next
	if r.conns[sessionID] == nil {
		r.conns[sessionID] = make(map[uint64]func())
	}
	r.conns[sessionID][id] = closer
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if m := r.conns[sessionID]; m != nil {
			delete(m, id)
			if len(m) == 0 {
				delete(r.conns, sessionID)
			}
		}
	}
}

// Close forcibly closes every live connection for a session and returns the
// number closed.
func (r *Registry) Close(sessionID uuid.UUID) int {
	r.mu.Lock()
	closers := make([]func(), 0)
	if m := r.conns[sessionID]; m != nil {
		for _, c := range m {
			closers = append(closers, c)
		}
		delete(r.conns, sessionID)
	}
	r.mu.Unlock()
	for _, c := range closers {
		c()
	}
	return len(closers)
}
