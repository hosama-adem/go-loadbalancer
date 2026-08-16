package loadbalancer

// Strategy determines which backend should receive
// the next request.
type Strategy interface {
	Next(backends []*Backend) *Backend
}
