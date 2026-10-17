package loadbalancer_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	loadbalancer "github.com/yornik/go-practice/exercises/25-loadbalancer"
)

func TestConcurrentTraffic(t *testing.T) {
	// Many clients at once, while health checks run, the clock moves and
	// Status is read. Run it with -race: it finds unsynchronized state.
	//
	// The backends: two good ones, one that always answers 500 and whose
	// health check flaps, and one that is down. Every request ends at a good
	// backend (200) or at the failing one (500): the good ones are never
	// unhealthy or ejected, and a GET that hits the dead one is retried.
	e := newEnv(t)
	good1, good2 := e.upstream("good-1"), e.upstream("good-2")
	failing := e.upstream("failing")
	failing.status.Store(http.StatusInternalServerError)
	dead := e.deadURL()

	c := e.config(good1.URL, good2.URL, failing.URL, dead)
	c.Strategy = loadbalancer.PowerOfTwo(nil)
	c.MaxRetries = 3
	c.EjectAfter = 2
	c.EjectFor = time.Second
	c.UnhealthyAfter = 2
	c.HealthyAfter = 2
	c.HealthTimeout = timeout
	lb := newLB(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		for i := 0; ctx.Err() == nil; i++ {
			failing.health.Store(int32([]int{200, 200, 503, 503}[i%4]))
			lb.CheckHealth(ctx)
			lb.Status()
			e.clock.Advance(300 * time.Millisecond)
		}
	}()

	const clients, perClient = 16, 25
	var ok, failed, other atomic.Int32
	var wg sync.WaitGroup
	for range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perClient {
				rec := httptest.NewRecorder()
				lb.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://lb.example/", nil))
				switch rec.Code {
				case http.StatusOK:
					ok.Add(1)
				case http.StatusInternalServerError:
					failed.Add(1)
				default:
					other.Add(1)
				}
			}
		}()
	}
	within(t, "the clients", wg.Wait)
	cancel()
	within(t, "the health checker", background.Wait)

	if n := other.Load(); n != 0 {
		t.Errorf("%d responses had a status other than 200 or 500", n)
	}
	// Every 200 came from a good backend and every 500 from the failing one:
	// no request was sent twice, none got lost.
	if got, want := good1.hits.Load()+good2.hits.Load(), ok.Load(); got != want {
		t.Errorf("the good backends got %d requests, but %d responses were 200", got, want)
	}
	if got, want := failing.hits.Load(), failed.Load(); got != want {
		t.Errorf("the failing backend got %d requests, but %d responses were 500", got, want)
	}
	for _, u := range []string{good1.URL, good2.URL, failing.URL, dead} {
		if got := status(t, lb, u).InFlight; got != 0 {
			t.Errorf("InFlight of %s = %d after all requests finished, want 0", u, got)
		}
	}
}
