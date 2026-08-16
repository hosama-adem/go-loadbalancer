package loadbalancer

import (
	"net/http/httputil"
	"net/url"
	"testing"
)

func TestBackendSetAlive(t *testing.T) {
	u, err := url.Parse("http://localhost:8081")
	if err != nil {
		t.Fatal(err)
	}

	backend := &Backend{
		URL:          u,
		ReverseProxy: httputil.NewSingleHostReverseProxy(u),
		alive:        true,
	}

	if !backend.IsAlive() {
		t.Fatal("expected backend to be alive")
	}

	backend.SetAlive(false)

	if backend.IsAlive() {
		t.Fatal("expected backend to be dead")
	}

	backend.SetAlive(true)

	if !backend.IsAlive() {
		t.Fatal("expected backend to be alive")
	}
}
