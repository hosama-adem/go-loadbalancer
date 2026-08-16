package loadbalancer

import "sync/atomic"

// RoundRobin implements the round-robin load-balancing strategy.
type RoundRobin struct {
	current uint64
}

// NewRoundRobin creates a new Round Robin strategy.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{
		current: ^uint64(0),
	}
}

// Next returns the next healthy backend.
func (r *RoundRobin) Next(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}

	start := int(
		atomic.AddUint64(&r.current, 1) %
			uint64(len(backends)),
	)

	for i := 0; i < len(backends); i++ {
		index := (start + i) % len(backends)

		if backends[index].IsAlive() {
			return backends[index]
		}
	}

	return nil
}
