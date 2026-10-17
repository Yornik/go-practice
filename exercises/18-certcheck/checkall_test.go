package certcheck_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	certcheck "github.com/yornik/go-practice/exercises/18-certcheck"
)

// gatedSite is a TLS server for many host names (t0.example.com,
// t1.example.com, ...), each with its own leaf certificate. The handshake for
// ti.example.com waits in the server until open is closed if hold(i) is
// true. inFlight and peak count the handshakes that are waiting there.
type gatedSite struct {
	*server
	leaves   []*x509.Certificate // leaves[i] is the certificate of ti.example.com
	targets  []certcheck.Target  // targets[i] is ti.example.com on this server
	open     chan struct{}
	inFlight atomic.Int32
	peak     atomic.Int32
}

// startGatedSite serves n host names. Leaf i expires at base+(40+10*i) days,
// so every result can be traced back to its target.
func startGatedSite(t *testing.T, n int, hold func(i int) bool) *gatedSite {
	t.Helper()
	f := pki()
	g := &gatedSite{open: make(chan struct{})}
	byName := map[string]int{}
	for i := range n {
		name := fmt.Sprintf("t%d.example.com", i)
		byName[name] = i
		g.leaves = append(g.leaves, f.leaf([]string{name}, base.Add(-30*day), base.Add(time.Duration(40+10*i)*day)))
	}
	g.server = startTLS(t, &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			i, ok := byName[hello.ServerName]
			if !ok {
				return nil, fmt.Errorf("no certificate for %q", hello.ServerName)
			}
			if hold(i) {
				trackPeak(&g.peak, g.inFlight.Add(1))
				defer g.inFlight.Add(-1)
				select {
				case <-g.open:
				case <-hello.Context().Done(): // the test is over
					return nil, hello.Context().Err()
				}
			}
			return tlsCert(g.leaves[i], f.inter), nil
		},
	})
	for i := range n {
		g.targets = append(g.targets, certcheck.Target{
			Name:       fmt.Sprintf("t%d", i),
			Addr:       g.addr,
			ServerName: fmt.Sprintf("t%d.example.com", i),
		})
	}
	return g
}

// trackPeak raises peak to cur if cur is larger. Safe for concurrent use.
func trackPeak(peak *atomic.Int32, cur int32) {
	for {
		p := peak.Load()
		if cur <= p || peak.CompareAndSwap(p, cur) {
			return
		}
	}
}

// checkResults checks that rs has one OK result per target of g, in order.
func (g *gatedSite) checkResults(t *testing.T, rs []certcheck.Result) {
	t.Helper()
	if len(rs) != len(g.targets) {
		t.Fatalf("CheckAll returned %d results, want %d", len(rs), len(g.targets))
	}
	for i, r := range rs {
		if r.Target != g.targets[i] || !r.NotAfter.Equal(g.leaves[i].NotAfter) {
			t.Errorf("result %d: Target %q, NotAfter %v; want Target %q, NotAfter %v",
				i, r.Target.Name, r.NotAfter.Format(time.DateOnly), g.targets[i].Name, g.leaves[i].NotAfter.Format(time.DateOnly))
		}
		if r.Status != certcheck.OK {
			t.Errorf("result %d (%s): Status = %v, want OK (Err: %s)", i, r.Target.Name, r.Status, errText(r.Err))
		}
	}
}

// Results come back in the order of the targets, not in the order the checks
// finish. Here t0 finishes last: its handshake is held until t1..t4 are done.
// With Concurrency 2, that also shows that a slow target holds up only its own
// slot: t1..t4 run one after the other in the second slot.
func TestCheckAllInputOrder(t *testing.T) {
	g := startGatedSite(t, 5, func(i int) bool { return i == 0 })
	c := newChecker(base)
	c.Concurrency = 2
	c.Timeout = 0 // means DefaultTimeout (10s), not "no time at all"

	ch := startCheckAll(t.Context(), c, g.targets)
	waitWhileRunning(t, "CheckAll", "t1..t4 are checked while t0 is still running (with Concurrency 2)", ch,
		func() bool { return g.ended.Load() == 4 })
	close(g.open)
	rs := wait(t, "CheckAll", ch)

	g.checkResults(t, rs)
}

// At most Concurrency checks run at the same time, and with enough targets
// CheckAll does use all of them.
func TestCheckAllConcurrencyLimit(t *testing.T) {
	cases := []struct {
		concurrency int
		want        int32
	}{
		{3, 3},
		{0, 1}, // below 1 means 1
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("Concurrency %d", tc.concurrency), func(t *testing.T) {
			g := startGatedSite(t, 7, func(int) bool { return true })
			c := newChecker(base)
			c.Concurrency = tc.concurrency
			c.Timeout = time.Minute // the held handshakes must not time out

			ch := startCheckAll(t.Context(), c, g.targets)
			waitWhileRunning(t, "CheckAll", fmt.Sprintf("%d checks wait for the server at the same time", tc.want), ch,
				func() bool { return g.inFlight.Load() >= tc.want })
			time.Sleep(window) // give a wrong solution time to start more
			if p := g.peak.Load(); p > tc.want {
				t.Errorf("%d checks ran at the same time, want at most %d", p, tc.want)
			}
			close(g.open)
			rs := wait(t, "CheckAll", ch)

			if p := g.peak.Load(); p > tc.want && !t.Failed() {
				t.Errorf("%d checks ran at the same time after the server let them go on, want at most %d", p, tc.want)
			}
			if c.Concurrency != tc.concurrency {
				t.Errorf("after CheckAll, Checker.Concurrency = %d, want %d: CheckAll must not modify the Checker", c.Concurrency, tc.concurrency)
			}
			g.checkResults(t, rs)
		})
	}
}

// Every target gets a result, whatever happened to it.
func TestCheckAllMixedResults(t *testing.T) {
	f := pki()
	good := serveChain(t, f.www, f.inter)
	expired := serveChain(t, f.old, f.oldInter)
	ssh := startSSH(t)
	ts := []certcheck.Target{
		{Name: "www", Addr: good.addr, ServerName: "www.example.com"},
		{Name: "shop", Addr: good.addr, ServerName: "shop.example.com"},
		{Name: "down", Addr: closedAddr(t), ServerName: "www.example.com"},
		{Name: "old", Addr: expired.addr, ServerName: "old.example.com"},
		{Name: "ssh", Addr: ssh.addr, ServerName: "www.example.com"},
	}
	want := []certcheck.Status{certcheck.OK, certcheck.Invalid, certcheck.Error, certcheck.Expired, certcheck.Error}
	c := newChecker(base)
	c.Concurrency = 3

	rs := wait(t, "CheckAll", startCheckAll(t.Context(), c, ts))

	if len(rs) != len(ts) {
		t.Fatalf("CheckAll returned %d results, want %d", len(rs), len(ts))
	}
	for i, r := range rs {
		if r.Target != ts[i] || r.Status != want[i] {
			t.Errorf("result %d = %s %v, want %s %v (Err: %s)", i, r.Target.Name, r.Status, ts[i].Name, want[i], errText(r.Err))
		}
	}
}

func TestCheckAllNoTargets(t *testing.T) {
	rs := wait(t, "CheckAll", startCheckAll(t.Context(), newChecker(base), nil))
	if len(rs) != 0 {
		t.Errorf("CheckAll(no targets) returned %d results, want 0", len(rs))
	}
}

// Ctrl-C: the running checks stop, no new connections are opened, and every
// target still gets a result.
func TestCheckAllCancel(t *testing.T) {
	srv := startSilent(t)
	c := newChecker(base)
	c.Timeout = time.Minute
	c.Concurrency = 2
	var ts []certcheck.Target
	for i := range 4 {
		ts = append(ts, certcheck.Target{Name: fmt.Sprintf("s%d", i), Addr: srv.addr, ServerName: "www.example.com"})
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := startCheckAll(ctx, c, ts)
	waitWhileRunning(t, "CheckAll", "2 checks are connected", ch, func() bool { return srv.accepted.Load() >= 2 })
	cancel()
	rs := wait(t, "CheckAll after cancel", ch) // Timeout is 1 minute: only ctx can end this in time

	if len(rs) != len(ts) {
		t.Fatalf("CheckAll returned %d results, want %d", len(rs), len(ts))
	}
	for i, r := range rs {
		if r.Target != ts[i] || r.Status != certcheck.Error || !errors.Is(r.Err, context.Canceled) {
			t.Errorf("result %d = %s %v (Err: %s); want %s ERROR with an Err that matches context.Canceled",
				i, r.Target.Name, r.Status, errText(r.Err), ts[i].Name)
		}
	}
	if n := srv.accepted.Load(); n != 2 {
		t.Errorf("the server accepted %d connections, want 2: no new connection after ctx is done", n)
	}
}
