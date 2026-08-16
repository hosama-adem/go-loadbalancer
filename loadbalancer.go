package loadbalancer

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// LoadBalancer distributes HTTP requests
// across multiple backend servers.
type LoadBalancer struct {
	pool     *ServerPool
	strategy Strategy
}

// New creates a new load balancer.
//
// Round Robin is used by default.
func New() *LoadBalancer {
	return &LoadBalancer{
		pool:     &ServerPool{},
		strategy: NewRoundRobin(),
	}
}

// AddBackend adds a backend server.
func (lb *LoadBalancer) AddBackend(rawURL string) error {
	backendURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf(
			"invalid backend URL %q: %w",
			rawURL,
			err,
		)
	}

	if backendURL.Host == "" {
		return fmt.Errorf(
			"backend URL %q does not contain a host",
			rawURL,
		)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	backend := &Backend{
		URL:          backendURL,
		ReverseProxy: proxy,
		alive:        true,
	}

	lb.pool.AddBackend(backend)

	return nil
}

// ServeHTTP implements http.Handler.
func (lb *LoadBalancer) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	backend := lb.strategy.Next(lb.pool.Backends())

	if backend == nil {
		http.Error(
			w,
			"Service unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	backend.ReverseProxy.ServeHTTP(w, r)
}
