package loadbalancer

import (
	"net/http/httputil"
	"net/url"
	"testing"
)

func TestServerPoolAddBackend(t *testing.T) {
	pool := &ServerPool{}

	u1, err := url.Parse("http://localhost:8081")
	if err != nil {
		t.Fatal(err)
	}

	u2, err := url.Parse("http://localhost:8082")
	if err != nil {
		t.Fatal(err)
	}

	backend1 := &Backend{
		URL:          u1,
		ReverseProxy: httputil.NewSingleHostReverseProxy(u1),
		alive:        true,
	}

	backend2 := &Backend{
		URL:          u2,
		ReverseProxy: httputil.NewSingleHostReverseProxy(u2),
		alive:        true,
	}

	pool.AddBackend(backend1)
	pool.AddBackend(backend2)

	backends := pool.Backends()

	if len(backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(backends))
	}
}
