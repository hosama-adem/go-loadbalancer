package loadbalancer

import (
	"net/http/httputil"
	"net/url"
	"sync"
)

// Backend represents a backend server.
type Backend struct {
	URL          *url.URL
	ReverseProxy *httputil.ReverseProxy

	mu    sync.RWMutex
	alive bool
}

// SetAlive updates the health status of the backend.
func (b *Backend) SetAlive(alive bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.alive = alive
}

// IsAlive returns whether the backend is currently healthy.
func (b *Backend) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.alive
}
