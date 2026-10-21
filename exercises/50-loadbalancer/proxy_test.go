package loadbalancer_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	loadbalancer "github.com/yornik/go-practice/exercises/50-loadbalancer"
)

// ----- New -----

func TestNewInvalidConfig(t *testing.T) {
	ok := []string{"http://10.0.0.7:8080"}
	tests := []struct {
		name string
		c    loadbalancer.Config
	}{
		{"no backends", loadbalancer.Config{}},
		{"backend without scheme", loadbalancer.Config{Backends: []string{"10.0.0.7:8080"}}},
		{"backend with ftp scheme", loadbalancer.Config{Backends: []string{"ftp://10.0.0.7"}}},
		{"backend without host", loadbalancer.Config{Backends: []string{"http://"}}},
		{"one bad backend among good ones", loadbalancer.Config{Backends: []string{"http://10.0.0.7:8080", "/api"}}},
		{"same backend twice", loadbalancer.Config{Backends: []string{"http://10.0.0.7:8080", "http://10.0.0.7:8080"}}},
		{"negative MaxRetries", loadbalancer.Config{Backends: ok, MaxRetries: -1}},
		{"negative EjectAfter", loadbalancer.Config{Backends: ok, EjectAfter: -1}},
		{"negative EjectFor", loadbalancer.Config{Backends: ok, EjectFor: -time.Second}},
		{"negative UnhealthyAfter", loadbalancer.Config{Backends: ok, UnhealthyAfter: -1}},
		{"negative HealthTimeout", loadbalancer.Config{Backends: ok, HealthTimeout: -time.Second}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			lb, err := loadbalancer.New(tc.c)
			if err == nil || lb != nil {
				t.Errorf("New(%s) = %v, %v; want nil and an error", tc.name, lb, err)
			}
		})
	}
}

func TestNewDefaults(t *testing.T) {
	t.Run("nil Strategy means round robin", func(t *testing.T) {
		e := newEnv(t)
		a, b := e.upstream("a"), e.upstream("b")
		c := e.config(a.URL, b.URL)
		c.Strategy = nil
		lb := newLB(t, c)
		var got []string
		for range 4 {
			got = append(got, get(t, lb, "/").Body.String())
		}
		if want := []string{"a", "b", "a", "b"}; !slices.Equal(got, want) {
			t.Errorf("4 requests went to %v, want %v", got, want)
		}
	})
	t.Run("nil Transport means http.DefaultTransport", func(t *testing.T) {
		e := newEnv(t)
		a := e.upstream("a")
		c := e.config(a.URL)
		c.Transport = nil
		checkResponse(t, get(t, newLB(t, c), "/"), http.StatusOK, "a")
	})
}

// ----- Proxying -----

func TestProxyForwardsRequestAndResponse(t *testing.T) {
	e := newEnv(t)
	type seen struct {
		method, uri, body, host, requestID, xff, xfh, xfp string
	}
	got := make(chan seen, 1)
	srv := e.server(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- seen{
			method: r.Method, uri: r.RequestURI, body: string(body), host: r.Host,
			requestID: r.Header.Get("X-Request-ID"),
			xff:       strings.Join(r.Header.Values("X-Forwarded-For"), ","),
			xfh:       r.Header.Get("X-Forwarded-Host"),
			xfp:       r.Header.Get("X-Forwarded-Proto"),
		}
		w.Header().Set("X-Upstream", "orders-1")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "created\n")
	}))
	lb := newLB(t, e.config(srv.URL))

	req := httptest.NewRequest(http.MethodPost, "http://shop.example.com/api/v1/orders?id=7&dry_run=true", strings.NewReader(`{"qty":1}`))
	req.Header.Set("X-Request-ID", "req-42")
	req.Header.Set("X-Forwarded-For", "203.0.113.66") // sent by the client: not trusted
	rec := do(t, lb, req)

	// The client gets the backend's response.
	checkResponse(t, rec, http.StatusCreated, "created\n")
	if h := rec.Header().Get("X-Upstream"); h != "orders-1" {
		t.Errorf("response header X-Upstream = %q, want %q", h, "orders-1")
	}

	// The backend gets the client's request.
	var s seen
	select {
	case s = <-got:
	default:
		t.Fatal("the backend received no request")
	}
	backendHost := strings.TrimPrefix(srv.URL, "http://")
	checks := []struct{ what, got, want string }{
		{"method", s.method, http.MethodPost},
		{"path and query", s.uri, "/api/v1/orders?id=7&dry_run=true"},
		{"body", s.body, `{"qty":1}`},
		{"Host (the backend's own host)", s.host, backendHost},
		{"X-Request-ID (copied)", s.requestID, "req-42"},
		{"X-Forwarded-For (only the client IP)", s.xff, "192.0.2.1"},
		{"X-Forwarded-Host (the Host the client asked for)", s.xfh, "shop.example.com"},
		{"X-Forwarded-Proto", s.xfp, "http"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("backend saw %s = %q, want %q", c.what, c.got, c.want)
		}
	}
}

func TestProxyPassesBackendErrors(t *testing.T) {
	// The LB passes every response on, also 4xx and 5xx. Only when no
	// response comes back at all does it answer itself.
	for _, code := range []int{http.StatusNotFound, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			e := newEnv(t)
			a := e.upstream("a")
			a.status.Store(int32(code))
			lb := newLB(t, e.config(a.URL))
			checkResponse(t, get(t, lb, "/"), code, "a")
		})
	}
}

func TestProxyBackendDown(t *testing.T) {
	// No response at all (connection refused): 502 Bad Gateway.
	e := newEnv(t)
	dead := e.deadURL()
	lb := newLB(t, e.config(dead))
	rec := get(t, lb, "/")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("GET via a backend that refuses connections: status %d, want 502", rec.Code)
	}
	if n := e.attempts(dead); n != 1 {
		t.Errorf("the LB sent %d requests to the dead backend, want 1", n)
	}
	if got := status(t, lb, dead).InFlight; got != 0 {
		t.Errorf("InFlight after the failed request = %d, want 0", got)
	}
}

func TestProxyLeastConnectionsWithSlowBackends(t *testing.T) {
	// Three slow backends: every request waits until the test releases it.
	// Requests are sent one at a time, so the in-flight counts are exact and
	// LeastConnections must fill the backends evenly.
	e := newEnv(t)
	a, b, c := e.upstream("a"), e.upstream("b"), e.upstream("c")
	for _, u := range []*upstream{a, b, c} {
		u.hold.Store(true)
	}
	lb := newLB(t, e.config(a.URL, b.URL, c.URL))

	var reqs []<-chan pending
	var wantBodies []string
	send := func(want string) {
		t.Helper()
		reqs = append(reqs, startGet(lb, "/slow"))
		wantBodies = append(wantBodies, want)
		e.waitArrival(t, want)
	}
	for _, want := range []string{"a", "b", "c", "a", "b", "c"} {
		send(want)
	}
	for _, u := range []*upstream{a, b, c} {
		if got := status(t, lb, u.URL).InFlight; got != 2 {
			t.Errorf("InFlight of %s = %d, want 2", u.name, got)
		}
	}

	// One of b's requests finishes; b now has the fewest.
	b.releaseOne(t)
	waitInFlight(t, lb, b.URL, 1)
	send("b")

	// Let everything finish.
	for range 2 {
		a.releaseOne(t)
		b.releaseOne(t)
		c.releaseOne(t)
	}
	bodies := map[string]int{}
	for _, ch := range reqs {
		rec := await(t, ch)
		if rec.Code != http.StatusOK {
			t.Errorf("a slow request got status %d, want 200", rec.Code)
		}
		bodies[rec.Body.String()]++
	}
	if bodies["a"] != 2 || bodies["b"] != 3 || bodies["c"] != 2 {
		t.Errorf("responses per backend = %v, want a:2 b:3 c:2", bodies)
	}
	for _, u := range []*upstream{a, b, c} {
		waitInFlight(t, lb, u.URL, 0)
	}
}

func TestProxyInFlightAfterBrokenResponseBody(t *testing.T) {
	// The backend sends the headers and part of the body, then breaks the
	// connection. ReverseProxy then aborts the LB's handler with
	// panic(http.ErrAbortHandler). The request is over, so InFlight must
	// drop back to 0 anyway.
	e := newEnv(t)
	srv := e.server(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "partial")
		_ = http.NewResponseController(w).Flush()
		panic(http.ErrAbortHandler) // the server closes the connection
	}))
	lb := newLB(t, e.config(srv.URL))
	front := e.server(lb) // a real server in front, as in production

	within(t, "GET through the LB", func() {
		res, err := front.Client().Get(front.URL + "/export")
		if err == nil {
			_, _ = io.ReadAll(res.Body) // fails half way; that is expected
			_ = res.Body.Close()
		}
	})
	waitInFlight(t, lb, srv.URL, 0)
}

// ----- Status -----

func TestStatusInitial(t *testing.T) {
	e := newEnv(t)
	urls := []string{"http://10.0.0.7:8080", "http://10.0.0.8:8080", "https://10.0.0.9:8443"}
	lb := newLB(t, e.config(urls...))
	var got []loadbalancer.BackendStatus
	within(t, "Status", func() { got = lb.Status() })
	var want []loadbalancer.BackendStatus
	for _, u := range urls {
		want = append(want, loadbalancer.BackendStatus{URL: u, Healthy: true})
	}
	if !slices.Equal(got, want) {
		t.Errorf("Status() of a new LB =\n%+v\nwant\n%+v", got, want)
	}
}
