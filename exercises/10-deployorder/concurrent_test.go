package deployorder_test

import (
	"slices"
	"sync"
	"testing"
	"time"
)

// A Graph does not change after New, so many goroutines can query it at once.
// Run with -race: a Graph that fills a cache on first use without a lock
// usually passes without -race and fails with it.
func TestConcurrentUse(t *testing.T) {
	g := mustNew(t, platform())
	wantWaves := [][]string{
		{"cert-manager", "kafka", "postgres", "redis"},
		{"auth", "ingress-nginx"},
		{"orders", "payments"},
		{"api-gateway"},
		{"web"},
	}
	wantOrder := slices.Concat(wantWaves...)
	wantDependents := []string{"api-gateway", "auth", "orders", "payments", "web"}
	wantWhy := []string{"web", "api-gateway", "auth", "postgres"}

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if w, err := g.Waves(); err != nil || !equalWaves(w, wantWaves) {
					t.Errorf("concurrent Waves() = %q, %v; want %q, nil", w, err, wantWaves)
					return
				}
				if o, err := g.Order(); err != nil || !slices.Equal(o, wantOrder) {
					t.Errorf("concurrent Order() = %q, %v; want %q, nil", o, err, wantOrder)
					return
				}
				if d, err := g.Dependents("postgres"); err != nil || !slices.Equal(d, wantDependents) {
					t.Errorf("concurrent Dependents(%q) = %q, %v; want %q, nil", "postgres", d, err, wantDependents)
					return
				}
				if c, ok := g.Why("web", "postgres"); !ok || !slices.Equal(c, wantWhy) {
					t.Errorf("concurrent Why(%q, %q) = %q, %v; want %q, true", "web", "postgres", c, ok, wantWhy)
					return
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("8 goroutines querying the Graph did not finish within 5s: is a method blocked on a lock?")
	}
}
