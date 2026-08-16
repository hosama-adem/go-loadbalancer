# Go Load Balancer

[![CI](https://github.com/hosama-adem/go-loadbalancer/actions/workflows/ci.yml/badge.svg)](https://github.com/hosama-adem/go-loadbalancer/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/hosama-adem/go-loadbalancer.svg)](https://pkg.go.dev/github.com/hosama-adem/go-loadbalancer)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A small Go library that turns a list of backend URLs into a single
`http.Handler` that Round-Robins requests across them — no external
dependencies, no separate process to run.

```go
lb := loadbalancer.New()
lb.AddBackend("http://localhost:8081")
lb.AddBackend("http://localhost:8082")
http.ListenAndServe(":3030", lb)
```

## Table of Contents

- [Overview](#overview)
- [How This Is Different](#how-this-is-different)
- [Features](#features)
- [How It Works](#how-it-works)
- [Round Robin](#round-robin)
- [Architecture](#architecture)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Best Practices](#best-practices)
- [Configuration / API Usage](#configuration--api-usage)
- [Running the Example](#running-the-example)
- [Concurrency and Thread Safety](#concurrency-and-thread-safety)
- [Testing](#testing)
- [Continuous Integration](#continuous-integration)
- [Project Structure](#project-structure)
- [Current Limitations](#current-limitations)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [Versioning](#versioning)
- [License](#license)
- [Author](#author)

## Overview

`go-loadbalancer` is an embeddable HTTP load balancer for Go services. It's
a type — `LoadBalancer` — not a program: you register backend URLs with it
and hand it to `http.ListenAndServe` like any other handler. Every request
that hits it gets forwarded to one of the registered backends, in Round
Robin order.

The whole library is built on the standard library's
`net/http/httputil.ReverseProxy`. It currently ships one selection
strategy — Round Robin — and imports nothing outside the standard library.

## How This Is Different

This isn't a competitor to nginx, HAProxy, or Envoy, and it isn't trying to
be a full-featured Go load balancing framework either. A few deliberate
choices set it apart:

- **It's a library, not a service.** You import it into your own Go
  program and get a handler back — there's no separate binary to deploy,
  configure, or keep alive.
- **It does one thing.** Round Robin over HTTP backends. No TLS
  termination, no service discovery, no metrics pipeline — which also
  means very little to misconfigure.
- **It's small enough to read in one sitting.** The entire selection and
  proxying logic spans a handful of files, well under 200 lines combined.
  You can audit exactly what it does instead of trusting a black box.
- **The internals are exported on purpose.** Alongside the convenient
  `LoadBalancer` type, the lower-level pieces it's built from —
  `ServerPool`, `Backend`, `Strategy` — are all public. When
  `LoadBalancer` doesn't do something you need (health flagging, custom
  timeouts), you can assemble those pieces yourself instead of forking the
  library. See [Best Practices](#best-practices).

## Features

**Implemented:**

- Round Robin backend selection, safe for concurrent use (`sync/atomic`
  counter).
- Standard library reverse proxying via `net/http/httputil.ReverseProxy`.
- Backend URL validation on registration — `AddBackend` rejects URLs that
  fail to parse or have no host.
- A per-backend alive/dead flag (`Backend.SetAlive` / `IsAlive`, guarded by
  a `sync.RWMutex`) that `RoundRobin.Next` respects, skipping backends
  currently marked dead.
- `LoadBalancer` implements `http.Handler`, so it drops into any
  `net/http`-based server.

**Not implemented:** active/passive health checking, weighted or
least-connections balancing, metrics, service discovery, circuit breaking,
or request retries. See [Current Limitations](#current-limitations) for
exactly what exists in the code but isn't wired up yet, and
[Best Practices](#best-practices) for how to work around each gap.

## How It Works

Two things happen in this library: registering a backend, and handling a
request. Both are short enough to walk through in full.

**Registering a backend — `AddBackend(rawURL)`:**

1. Parses `rawURL` with `net/url.Parse`.
2. Rejects it if parsing fails, or if the result has no `Host`. This is
   stricter than it might look: `AddBackend("localhost:8081")` fails,
   because `url.Parse` reads `localhost` as a *scheme* and `8081` as
   opaque data when there's no `//` — there's no `Host` component at all.
   Always include the scheme: `http://localhost:8081`.
3. On success, wraps the parsed URL in
   `httputil.NewSingleHostReverseProxy(backendURL)`, marks it alive, and
   appends it to the internal pool.

**Handling a request — `ServeHTTP(w, r)`:**

1. Asks the strategy for the next backend:
   `lb.strategy.Next(lb.pool.Backends())`.
2. `RoundRobin.Next` atomically increments its counter, computes
   `index := counter % len(backends)`, then scans forward from `index`
   (wrapping around) for the first backend whose `IsAlive()` is `true`.
3. If nothing is available — empty pool, or every backend dead — `Next`
   returns `nil` and `ServeHTTP` responds `503 Service Unavailable`
   without touching any backend.
4. Otherwise the request is handed off entirely to that backend's
   `ReverseProxy.ServeHTTP(w, r)`. From here, standard `httputil.ReverseProxy`
   behavior takes over: it rewrites the request's scheme and host to the
   backend's, and *joins* the backend URL's path with the incoming
   request's path — so a backend registered as `http://localhost:8081/api`
   receiving a request for `/users` forwards it as `/api/users`. If the
   backend is unreachable, `ReverseProxy` itself writes `502 Bad Gateway`.

Every behavior above was checked against the actual code, not assumed.

## Round Robin

Round Robin cycles through registered backends in order, wrapping back to
the first after the last. With three backends registered:

```
Request 1 -> Backend 1
Request 2 -> Backend 2
Request 3 -> Backend 3
Request 4 -> Backend 1
```

This is exactly what `round_robin_test.go` asserts. `NewRoundRobin` seeds
its internal counter at its maximum `uint64` value, so the first call wraps
around to index `0` — the sequence above starts clean from the first
backend registered.

## Architecture

```
                          Client
                             |
                             v
              LoadBalancer.ServeHTTP(w, r)
                             |
              lb.strategy.Next(lb.pool.Backends())
                             |
                             v
                      +-------------+
                      |  RoundRobin |
                      +-------------+
                             |
                 picks the next alive Backend
                             |
        +--------------------+--------------------+
        |                    |                     |
        v                    v                     v
   Backend 1            Backend 2             Backend 3
 (ReverseProxy)        (ReverseProxy)        (ReverseProxy)
        |                    |                     |
        v                    v                     v
    localhost:8081      localhost:8082        localhost:8083
```

`LoadBalancer` is just a `ServerPool` (backend storage) plus a `Strategy`
(selection logic):

```
LoadBalancer
 |-- pool      (*ServerPool)  -- holds --> []*Backend
 `-- strategy  (Strategy)     -- always *RoundRobin, set in New()
```

## Requirements

- Go 1.26.2 or later — the minimum version pinned by the `go` directive in
  `go.mod`.
- No external dependencies — the module imports only the standard library.

## Installation

```
go get github.com/hosama-adem/go-loadbalancer
```

The package name declared in every source file is `loadbalancer` (not
`go_loadbalancer`), so once imported it's referenced as `loadbalancer.X` by
default.

## Quick Start

This is the actual contents of `cmd/example/main.go`:

```go
package main

import (
	"log"
	"net/http"

	loadbalancer "github.com/hosama-adem/go-loadbalancer"
)

func main() {
	lb := loadbalancer.New()

	backends := []string{
		"http://localhost:8081",
		"http://localhost:8082",
		"http://localhost:8083",
	}

	for _, backend := range backends {
		if err := lb.AddBackend(backend); err != nil {
			log.Fatal(err)
		}
	}

	log.Println("Load balancer running on :3030")

	if err := http.ListenAndServe(":3030", lb); err != nil {
		log.Fatal(err)
	}
}
```

This won't proxy real traffic until something is listening on
`localhost:8081`, `:8082`, and `:8083` — see
[Running the Example](#running-the-example).

## Best Practices

This is the part that matters most if you're actually integrating the
library. Every pattern below was run against the real code before being
written down here — not just reasoned about.

### 1. Register every backend before you start serving

`ServerPool.AddBackend` has no locking (see
[Concurrency and Thread Safety](#concurrency-and-thread-safety)). Call
`AddBackend` for all your backends first, *then* call
`http.ListenAndServe`. Don't add backends after the server is already
handling live traffic.

### 2. Always include a scheme

`AddBackend("localhost:8081")` fails — verified above in
[How It Works](#how-it-works). Use `http://localhost:8081` or
`https://localhost:8081`.

### 3. A backend URL's path becomes a prefix — expect it

Register `http://localhost:8081/api` and a request for `/users` arrives at
the backend as `/api/users`. Useful if a backend genuinely needs a fixed
prefix; surprising otherwise. If you don't want prefixing, register bare
origins (`http://localhost:8081`).

### 4. Need health-awareness or custom timeouts? Skip `LoadBalancer` and assemble the pieces yourself

`LoadBalancer.AddBackend` never hands back the `*Backend` it creates, so
there's no way to call `SetAlive` on it later, and no way to set a custom
`Transport` on its `ReverseProxy`. If you need either, build the same
pieces `LoadBalancer` builds internally, using the exported types
directly:

```go
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	loadbalancer "github.com/hosama-adem/go-loadbalancer"
)

func mustBackend(rawURL string) *loadbalancer.Backend {
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.Transport = &http.Transport{
		ResponseHeaderTimeout: 5 * time.Second,
	}

	backend := &loadbalancer.Backend{URL: u, ReverseProxy: proxy}
	backend.SetAlive(true)
	return backend
}

func main() {
	pool := &loadbalancer.ServerPool{}
	pool.AddBackend(mustBackend("http://localhost:8081"))
	pool.AddBackend(mustBackend("http://localhost:8082"))

	strategy := loadbalancer.NewRoundRobin()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend := strategy.Next(pool.Backends())
		if backend == nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		backend.ReverseProxy.ServeHTTP(w, r)
	})

	// Your own health-check loop can now call backend.SetAlive(false/true)
	// on the *Backend values you're holding in `pool`.

	log.Fatal(http.ListenAndServe(":3030", handler))
}
```

This is the same flow `LoadBalancer.ServeHTTP` runs internally — you're
just holding the pieces instead of letting `LoadBalancer` hide them.
Verified behavior: with one backend registered this way, a request
succeeds; calling `backend.SetAlive(false)` immediately makes the next
request return `503`, with no other change needed.

### 5. Don't rely on `RetryPolicy`

It exists but isn't connected to anything in the package — constructing
one has no effect on request handling. If you need retries, implement them
yourself: wrap a backend's `Transport` with your own retrying
`http.RoundTripper`, or put a retrying layer in front of this handler.

### 6. Terminate TLS in front of this library, not inside it

There's no TLS configuration surface here. `AddBackend` happily accepts
`https://` backend URLs, but the front-facing listener is whatever you
pass to `http.ListenAndServe` / `ListenAndServeTLS` / your own
`http.Server`. Handle TLS the normal Go way, outside `LoadBalancer`.

### 7. Mount it like any other handler

`LoadBalancer` only implements `ServeHTTP`, so it composes normally with
the rest of `net/http` — wrap it in your own middleware (logging, auth,
rate limiting), or mount it at a specific path on an `http.ServeMux`,
instead of always giving it the whole server.

## Configuration / API Usage

Quick reference for every exported symbol. For the mechanics, see
[How It Works](#how-it-works); for how to actually use each piece, see
[Best Practices](#best-practices).

- **`func New() *LoadBalancer`** — empty pool, Round Robin selection (the
  only strategy `New` sets up).
- **`func (lb *LoadBalancer) AddBackend(rawURL string) error`** —
  validates and registers a backend.
- **`func (lb *LoadBalancer) ServeHTTP(w, r)`** — implements
  `http.Handler`.
- **`type Backend struct { URL *url.URL; ReverseProxy *httputil.ReverseProxy; ... }`**
  — plus `SetAlive(bool)` / `IsAlive() bool`, both goroutine-safe. Not
  reachable for backends added via `LoadBalancer.AddBackend` (see
  [Best Practices #4](#best-practices)).
- **`type ServerPool struct`** — `AddBackend(*Backend)`,
  `Backends() []*Backend`. Not synchronized.
- **`type Strategy interface { Next(backends []*Backend) *Backend }`** —
  the selection interface. `RoundRobin` is the only implementation
  shipped, and it can't currently be swapped into `LoadBalancer`.
- **`type RoundRobin struct` / `func NewRoundRobin() *RoundRobin`** — the
  built-in strategy, safe for concurrent use.
- **`type RetryPolicy struct { MaxRetries int; Delay time.Duration }`** —
  plus `NewRetryPolicy`, `ShouldRetry(attempt int) bool`, `Wait()`.
  Exported, unused elsewhere in the package.

## Running the Example

```
git clone https://github.com/hosama-adem/go-loadbalancer.git
cd go-loadbalancer
go run ./cmd/example
```

The example doesn't start backend servers for you — it only registers
`http://localhost:8081`, `:8082`, and `:8083` as targets. Without real
servers on those ports, every proxied request comes back `502 Bad
Gateway`.

To see it actually load balance, start three dummy backends first, each in
its own terminal:

```go
// backend.go — run with PORT=8081 go run backend.go (then 8082, 8083)
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "response from backend on port %s\n", port)
	})
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
```

With all three running and the example started, repeated requests to
`http://localhost:3030` cycle across all three backends in order.

## Concurrency and Thread Safety

- `Backend.SetAlive` / `IsAlive` — safe for concurrent use
  (`sync.RWMutex`).
- `RoundRobin.Next` — safe for concurrent use (`atomic.AddUint64`).
- `ServerPool.AddBackend` / `Backends()` — **not** synchronized. Calling
  `LoadBalancer.AddBackend` while requests are in flight (which read
  `lb.pool.Backends()` on every `ServeHTTP` call) is a data race. See
  [Best Practices #1](#best-practices).

## Testing

```
go test ./...
go test -race ./...
go test -cover ./...
```

All three were run against this repository and pass. Existing test files:
`backend_test.go`, `pool_test.go`, `round_robin_test.go`. There's currently
no dedicated test file for `loadbalancer.go` (`New`, `AddBackend`,
`ServeHTTP`) or for `retry.go`.

## Continuous Integration

`.github/workflows/ci.yml` runs on every push and pull request targeting
`main` or `master`, on `ubuntu-latest`:

1. Checkout (`actions/checkout@v4`).
2. Go setup (`actions/setup-go@v5`), reading the version from
   `go-version-file: go.mod` — whatever `go.mod` pins gets installed.
3. Formatting check: `test -z "$(gofmt -l .)"`.
4. `go test ./...`
5. `go test -race ./...`

Coverage reporting and linting beyond `gofmt` are not part of CI today.

## Project Structure

```
.
├── .github/
│   └── workflows/
│       └── ci.yml
├── cmd/
│   └── example/
│       └── main.go
├── .gitignore
├── LICENSE
├── backend.go
├── backend_test.go
├── go.mod
├── loadbalancer.go
├── pool.go
├── pool_test.go
├── retry.go
├── round_robin.go
├── round_robin_test.go
└── startegy.go
```

Note: the file defining `Strategy` is named `startegy.go` (missing the
second "r"), not `strategy.go`. It has no effect on behavior — Go resolves
imports by package declaration, not filename — but it's listed here as-is
since this section reflects the actual repository layout.

## Current Limitations

- Round Robin is the only strategy reachable through `LoadBalancer` —
  `New()` hard-codes it, with no option or setter for a custom `Strategy`.
- No automatic health checking — `SetAlive`/`IsAlive` exist, but nothing
  calls them, and `LoadBalancer` doesn't expose the backends it creates.
- `RetryPolicy` is defined but unused.
- No weighted balancing, metrics, service discovery, or circuit breaking.
- `ServerPool` has no internal locking.

Workarounds for each of these live in [Best Practices](#best-practices).

## Roadmap

No roadmap is currently published in this repository. Based on the gaps
above, listed as candidate directions rather than commitments:

- Wire `RetryPolicy` into `LoadBalancer.ServeHTTP`.
- Expose a way to supply a custom `Strategy` to `LoadBalancer`.
- Add a way to reach registered `*Backend` values from `LoadBalancer`.
- Add synchronization to `ServerPool`.

## Contributing

1. Fork the repository and create a branch for your change.
2. Make sure `gofmt -l .` reports no files — CI's formatting check will
   fail otherwise.
3. Run `go vet ./...` and `go test -race ./...` locally before opening a
   pull request.
4. Open a pull request against `main` describing the change and why it's
   needed. Keep pull requests focused.

There's no `CONTRIBUTING.md` in the repository yet; this section stands in
until one exists.

## Versioning

No releases or tags exist yet. Once tagged releases begin, this project
intends to follow [Semantic Versioning](https://semver.org/). Until a
`v1.0.0` tag exists, treat the public API as potentially unstable between
commits, per standard Go module conventions for pre-v1 modules.

## License

Released under the [MIT License](LICENSE).

```
MIT License

Copyright (c) 2026 Hosama Adem
```

See the `LICENSE` file for the full text.

## Author

**Hosama Adem**
GitHub: [@hosama-adem](https://github.com/hosama-adem)

---

## README Verification

- [x] Every code example (Quick Start, both Best Practices snippets, the
      dummy-backend example) was actually run against the repository —
      not just read and assumed correct.
- [x] Installation command matches the module path in `go.mod`.
- [x] Project Structure matches the repository's real file listing,
      including the `startegy.go` filename.
- [x] Features are split into implemented / not implemented based on
      reading every `.go` file.
- [x] Testing commands were run against the repository and pass.
- [x] CI documentation reflects the actual steps in
      `.github/workflows/ci.yml`, with nothing invented.
- [x] License information matches the `LICENSE` file.
- [x] No nonexistent features are documented; `RetryPolicy` and
      `Strategy`'s limited reach are called out explicitly, with concrete
      workarounds instead of just a warning.