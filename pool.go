package loadbalancer

// ServerPool stores all registered backend servers.
type ServerPool struct {
	backends []*Backend
}

// AddBackend adds a backend to the pool.
func (s *ServerPool) AddBackend(backend *Backend) {
	s.backends = append(s.backends, backend)
}

// Backends returns all registered backends.
func (s *ServerPool) Backends() []*Backend {
	return s.backends
}
