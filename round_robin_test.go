package loadbalancer

import (
	"net/http/httputil"
	"net/url"
	"testing"
)

func createTestBackend(
	t *testing.T,
	rawURL string,
) *Backend {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}

	return &Backend{
		URL:          u,
		ReverseProxy: httputil.NewSingleHostReverseProxy(u),
		alive:        true,
	}
}

func TestRoundRobin(t *testing.T) {
	backends := []*Backend{
		createTestBackend(t, "http://backend-1"),
		createTestBackend(t, "http://backend-2"),
		createTestBackend(t, "http://backend-3"),
	}

	strategy := NewRoundRobin()

	expected := []string{
		"http://backend-1",
		"http://backend-2",
		"http://backend-3",
		"http://backend-1",
		"http://backend-2",
		"http://backend-3",
	}

	for _, expectedURL := range expected {
		backend := strategy.Next(backends)

		if backend == nil {
			t.Fatal("expected backend, got nil")
		}

		if backend.URL.String() != expectedURL {
			t.Errorf(
				"expected %s, got %s",
				expectedURL,
				backend.URL.String(),
			)
		}
	}
}

func TestRoundRobinSkipsDeadBackend(t *testing.T) {
	backends := []*Backend{
		createTestBackend(t, "http://backend-1"),
		createTestBackend(t, "http://backend-2"),
		createTestBackend(t, "http://backend-3"),
	}

	backends[1].SetAlive(false)

	strategy := NewRoundRobin()

	for i := 0; i < 4; i++ {
		backend := strategy.Next(backends)

		if backend == nil {
			t.Fatal("expected backend, got nil")
		}

		t.Logf("Request %d -> %s", i+1, backend.URL.String())
	}
}
