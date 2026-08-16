package loadbalancer

import "time"

// RetryPolicy defines how failed requests should be retried.
type RetryPolicy struct {
	MaxRetries int
	Delay      time.Duration
}

// NewRetryPolicy creates a retry policy.
func NewRetryPolicy(
	maxRetries int,
	delay time.Duration,
) *RetryPolicy {
	return &RetryPolicy{
		MaxRetries: maxRetries,
		Delay:      delay,
	}
}

// ShouldRetry returns true if another retry is allowed.
func (r *RetryPolicy) ShouldRetry(attempt int) bool {
	return attempt < r.MaxRetries
}

// Wait pauses before the next retry.
func (r *RetryPolicy) Wait() {
	if r.Delay > 0 {
		time.Sleep(r.Delay)
	}
}
