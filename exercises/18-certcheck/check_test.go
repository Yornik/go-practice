package certcheck_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	certcheck "github.com/yornik/go-practice/exercises/18-certcheck"
)

// --- The happy path -------------------------------------------------------

func TestCheckValidChain(t *testing.T) {
	f := pki()
	srv := serveChain(t, f.www, f.inter)
	tg := certcheck.Target{Name: "www", Addr: srv.addr, ServerName: "www.example.com"}

	res := runCheck(t, t.Context(), newChecker(base), tg)

	if res.Status != certcheck.OK {
		t.Fatalf("Check(%+v).Status = %v, want OK (Err: %s)", tg, res.Status, errText(res.Err))
	}
	if res.Err != nil {
		t.Errorf("Err = %v, want nil", res.Err)
	}
	if res.Target != tg {
		t.Errorf("Target = %+v, want the target as passed in: %+v", res.Target, tg)
	}
	// The leaf expires before the intermediate, so it sets NotAfter.
	if !res.NotAfter.Equal(f.www.NotAfter) {
		t.Errorf("NotAfter = %v, want %v (the leaf's NotAfter)", res.NotAfter, f.www.NotAfter)
	}
	if want := 60 * day; res.Remaining != want {
		t.Errorf("Remaining = %v, want %v", res.Remaining, want)
	}
	if want := "CN=www.example.com"; res.Subject != want {
		t.Errorf("Subject = %q, want %q", res.Subject, want)
	}
	if want := "CN=Test Intermediate CA,O=go-practice"; res.Issuer != want {
		t.Errorf("Issuer = %q, want %q", res.Issuer, want)
	}
	if want := []string{"www.example.com", "example.com"}; !slices.Equal(res.DNSNames, want) {
		t.Errorf("DNSNames = %q, want %q", res.DNSNames, want)
	}
}

// --- Thresholds -----------------------------------------------------------

// TestCheckThresholds serves the same chain every time and moves the clock.
// The leaf expires at base+60d, before anything else in the chain.
// Warn is 30 days and Critical is 7 days, unless the case says otherwise.
func TestCheckThresholds(t *testing.T) {
	f := pki()
	srv := serveChain(t, f.www, f.inter)
	tg := certcheck.Target{Name: "www", Addr: srv.addr, ServerName: "www.example.com"}

	cases := []struct {
		name      string
		remaining time.Duration // Now = the leaf's NotAfter - remaining
		noLimits  bool          // Warn and Critical are 0
		want      certcheck.Status
	}{
		{"60 days left", 60 * day, false, certcheck.OK},
		{"exactly Warn left is still OK", 30 * day, false, certcheck.OK},
		{"1s less than Warn", 30*day - time.Second, false, certcheck.Warning},
		{"exactly Critical left is a Warning", 7 * day, false, certcheck.Warning},
		{"1s less than Critical", 7*day - time.Second, false, certcheck.Critical},
		// A certificate is valid up to and including its NotAfter second.
		{"at NotAfter it is still valid", 0, false, certcheck.Critical},
		{"1s after NotAfter", -time.Second, false, certcheck.Expired},
		{"3 days after NotAfter", -3 * day, false, certcheck.Expired},
		{"zero thresholds never warn", time.Hour, true, certcheck.OK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newChecker(f.www.NotAfter.Add(-tc.remaining))
			if tc.noLimits {
				c.Warn, c.Critical = 0, 0
			}
			res := runCheck(t, t.Context(), c, tg)

			if res.Status != tc.want {
				t.Errorf("Status = %v, want %v (Remaining %v, Err: %s)", res.Status, tc.want, res.Remaining, errText(res.Err))
			}
			if res.Remaining != tc.remaining {
				t.Errorf("Remaining = %v, want %v", res.Remaining, tc.remaining)
			}
			if !res.NotAfter.Equal(f.www.NotAfter) {
				t.Errorf("NotAfter = %v, want %v", res.NotAfter, f.www.NotAfter)
			}
			if tc.want != certcheck.Expired {
				// The chain verifies; a warning is not an error.
				if res.Err != nil {
					t.Errorf("Err = %v, want nil", res.Err)
				}
				return
			}
			// Verification fails for an expired leaf. Err keeps the x509 error.
			var invalid x509.CertificateInvalidError
			if !errors.As(res.Err, &invalid) || invalid.Reason != x509.Expired {
				t.Errorf("Err = %v (%T), want an x509.CertificateInvalidError with Reason x509.Expired", res.Err, res.Err)
			}
		})
	}
}

// --- Expiry anywhere in the chain -----------------------------------------

// The leaf is valid for 60 more days, but the intermediate it was signed with
// expired 2 days ago. Clients reject the chain. The status says why.
func TestCheckExpiredIntermediate(t *testing.T) {
	f := pki()
	srv := serveChain(t, f.old, f.oldInter)
	tg := certcheck.Target{Name: "old", Addr: srv.addr, ServerName: "old.example.com"}

	res := runCheck(t, t.Context(), newChecker(base), tg)

	if res.Status != certcheck.Expired {
		t.Errorf("Status = %v, want EXPIRED (Err: %s)", res.Status, errText(res.Err))
	}
	if !res.NotAfter.Equal(f.oldInter.NotAfter) {
		t.Errorf("NotAfter = %v, want %v (the intermediate's NotAfter, the earliest in the chain)", res.NotAfter, f.oldInter.NotAfter)
	}
	if want := -2 * day; res.Remaining != want {
		t.Errorf("Remaining = %v, want %v", res.Remaining, want)
	}
	if res.Err == nil {
		t.Errorf("Err = nil, want the verification error")
	}
	// Subject and Issuer always describe the leaf.
	if want := "CN=old.example.com"; res.Subject != want {
		t.Errorf("Subject = %q, want %q", res.Subject, want)
	}
	if want := "CN=Old Intermediate CA,O=go-practice"; res.Issuer != want {
		t.Errorf("Issuer = %q, want %q", res.Issuer, want)
	}
}

// The server sends an extra certificate that expired 10 days ago: an old
// cross-signed version of the root. Verification still succeeds, because the
// chain leaf -> intermediate -> root does not need it. Older clients may
// still try that path and fail, as in the AddTrust expiry of 2020. The rule
// is simple: NotAfter is the earliest NotAfter of everything the server sent.
func TestCheckExpiredExtraCertificate(t *testing.T) {
	f := pki()
	srv := serveChain(t, f.www, f.inter, f.cross)
	tg := certcheck.Target{Name: "www", Addr: srv.addr, ServerName: "www.example.com"}

	res := runCheck(t, t.Context(), newChecker(base), tg)

	if res.Status != certcheck.Expired {
		t.Errorf("Status = %v, want EXPIRED", res.Status)
	}
	if res.Err != nil {
		t.Errorf("Err = %v, want nil: the chain verifies", res.Err)
	}
	if !res.NotAfter.Equal(f.cross.NotAfter) {
		t.Errorf("NotAfter = %v, want %v (the extra certificate's NotAfter)", res.NotAfter, f.cross.NotAfter)
	}
	if want := -10 * day; res.Remaining != want {
		t.Errorf("Remaining = %v, want %v", res.Remaining, want)
	}
	if want := "CN=www.example.com"; res.Subject != want {
		t.Errorf("Subject = %q, want %q", res.Subject, want)
	}
}

// --- Chains that do not verify --------------------------------------------

func TestCheckInvalid(t *testing.T) {
	f := pki()
	full := serveChain(t, f.www, f.inter)
	leafOnly := serveChain(t, f.www)

	cases := []struct {
		name       string
		addr       string
		serverName string
		roots      *x509.CertPool
		now        time.Time
		wantErr    any // a pointer to the x509 error type Err must match with errors.As
	}{
		{
			name: "unknown authority", addr: full.addr, serverName: "www.example.com",
			roots: f.otherRoots, now: base,
			wantErr: new(x509.UnknownAuthorityError),
		},
		{
			// A common misconfiguration: the server sends only its own
			// certificate. Browsers often cope; curl and Go clients do not.
			name: "intermediate missing", addr: leafOnly.addr, serverName: "www.example.com",
			roots: f.roots, now: base,
			wantErr: new(x509.UnknownAuthorityError),
		},
		{
			name: "host name mismatch", addr: full.addr, serverName: "shop.example.com",
			roots: f.roots, now: base,
			wantErr: new(x509.HostnameError),
		},
		{
			// Valid from base-30d. A wrong clock or a certificate that was
			// issued for the future. x509 reports it with Reason x509.Expired,
			// but nothing has expired: the status is INVALID.
			name: "not valid yet", addr: full.addr, serverName: "www.example.com",
			roots: f.roots, now: base.Add(-31 * day),
			wantErr: new(x509.CertificateInvalidError),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newChecker(tc.now)
			c.Roots = tc.roots
			tg := certcheck.Target{Name: "www", Addr: tc.addr, ServerName: tc.serverName}

			res := runCheck(t, t.Context(), c, tg)

			if res.Status != certcheck.Invalid {
				t.Errorf("Status = %v, want INVALID (Err: %s)", res.Status, errText(res.Err))
			}
			if !errors.As(res.Err, tc.wantErr) {
				want := strings.TrimPrefix(fmt.Sprintf("%T", tc.wantErr), "*")
				t.Errorf("Err = %v (%T), want an error that matches %s with errors.As", res.Err, res.Err, want)
			}
			// Expiry is reported even though the chain does not verify.
			if !res.NotAfter.Equal(f.www.NotAfter) {
				t.Errorf("NotAfter = %v, want %v", res.NotAfter, f.www.NotAfter)
			}
			if want := f.www.NotAfter.Sub(tc.now); res.Remaining != want {
				t.Errorf("Remaining = %v, want %v", res.Remaining, want)
			}
			if want := "CN=www.example.com"; res.Subject != want {
				t.Errorf("Subject = %q, want %q", res.Subject, want)
			}
		})
	}
}

// --- Server name and SNI --------------------------------------------------

func TestCheckServerName(t *testing.T) {
	f := pki()

	t.Run("empty ServerName defaults to the host of Addr", func(t *testing.T) {
		// The leaf is valid for the IP address 127.0.0.1.
		srv := serveChain(t, f.www, f.inter)
		tg := certcheck.Target{Name: "by-ip", Addr: srv.addr}

		res := runCheck(t, t.Context(), newChecker(base), tg)

		if res.Status != certcheck.OK {
			t.Errorf("Status = %v, want OK (Err: %s)", res.Status, errText(res.Err))
		}
		if res.Target != tg {
			t.Errorf("Target = %+v, want the target as passed in: %+v", res.Target, tg)
		}
	})

	t.Run("the default host name is verified too", func(t *testing.T) {
		// This leaf is valid for shop.example.com only, not for 127.0.0.1.
		shop := f.leaf([]string{"shop.example.com"}, base.Add(-30*day), base.Add(60*day))
		srv := serveChain(t, shop, f.inter)
		tg := certcheck.Target{Name: "by-ip", Addr: srv.addr}

		res := runCheck(t, t.Context(), newChecker(base), tg)

		if res.Status != certcheck.Invalid {
			t.Errorf("Status = %v, want INVALID: the certificate is not valid for 127.0.0.1 (Err: %s)", res.Status, errText(res.Err))
		}
		var hostErr x509.HostnameError
		if !errors.As(res.Err, &hostErr) {
			t.Errorf("Err = %v (%T), want an x509.HostnameError", res.Err, res.Err)
		}
	})

	t.Run("ServerName is sent as SNI", func(t *testing.T) {
		// One IP address, two sites, as on an ingress controller: the server
		// picks the certificate by the name the client sends (SNI). Without
		// a name it sends the certificate of a.example.com.
		a := f.leaf([]string{"a.example.com"}, base.Add(-30*day), base.Add(20*day))
		b := f.leaf([]string{"b.example.com"}, base.Add(-30*day), base.Add(40*day))
		certs := map[string]*tls.Certificate{
			"":              tlsCert(a, f.inter),
			"a.example.com": tlsCert(a, f.inter),
			"b.example.com": tlsCert(b, f.inter),
		}
		srv := startTLS(t, &tls.Config{
			GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
				if c, ok := certs[hello.ServerName]; ok {
					return c, nil
				}
				return nil, fmt.Errorf("no certificate for %q", hello.ServerName)
			},
		})
		tg := certcheck.Target{Name: "b", Addr: srv.addr, ServerName: "b.example.com"}

		res := runCheck(t, t.Context(), newChecker(base), tg)

		if res.Status != certcheck.OK || !res.NotAfter.Equal(b.NotAfter) {
			t.Errorf("Status = %v, NotAfter = %v; want OK, %v: the certificate of b.example.com (Err: %s)",
				res.Status, res.NotAfter, b.NotAfter, errText(res.Err))
		}
	})
}

// --- No certificate at all ------------------------------------------------

func TestCheckConnectionErrors(t *testing.T) {
	ssh := startSSH(t)
	cases := []struct {
		name string
		addr string
	}{
		{"address without port", "127.0.0.1"},
		{"connection refused", closedAddr(t)},
		{"not a TLS server", ssh.addr},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tg := certcheck.Target{Name: "x", Addr: tc.addr, ServerName: "www.example.com"}

			res := runCheck(t, t.Context(), newChecker(base), tg)

			if res.Status != certcheck.Error {
				t.Errorf("Check(Addr %q).Status = %v, want ERROR", tc.addr, res.Status)
			}
			if res.Err == nil {
				t.Errorf("Err = nil, want the error")
			}
			if res.Target != tg {
				t.Errorf("Target = %+v, want %+v", res.Target, tg)
			}
			if !res.NotAfter.IsZero() || res.Remaining != 0 || res.Subject != "" || res.Issuer != "" || res.DNSNames != nil {
				t.Errorf("certificate fields = %v, %v, %q, %q, %q; want all zero: there is no certificate",
					res.NotAfter, res.Remaining, res.Subject, res.Issuer, res.DNSNames)
			}
		})
	}
}

// A server that accepts the TCP connection and then never answers the
// handshake must not hold the check longer than Timeout.
func TestCheckTimeout(t *testing.T) {
	srv := startSilent(t)
	c := newChecker(base)
	c.Timeout = 200 * time.Millisecond
	tg := certcheck.Target{Name: "silent", Addr: srv.addr, ServerName: "www.example.com"}

	begin := time.Now()
	res := runCheck(t, t.Context(), c, tg) // fails if Check takes longer than 10s
	elapsed := time.Since(begin)

	if res.Status != certcheck.Error || res.Err == nil {
		t.Errorf("Status = %v, Err = %v; want ERROR and the timeout error", res.Status, res.Err)
	}
	// 200ms plus a wide margin for a busy machine. Ignoring Timeout and
	// waiting DefaultTimeout (10s) is far beyond it.
	if elapsed > 5*time.Second {
		t.Errorf("Check returned after %v, want about 200ms: Timeout is 200ms", elapsed.Round(time.Millisecond))
	}
}

// Cancelling ctx stops a check that is waiting for the handshake, long
// before Timeout.
func TestCheckCancel(t *testing.T) {
	srv := startSilent(t)
	c := newChecker(base)
	c.Timeout = time.Minute
	tg := certcheck.Target{Name: "silent", Addr: srv.addr, ServerName: "www.example.com"}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	ch := startCheck(ctx, c, tg)
	waitWhileRunning(t, "Check", "the server accepted the connection", ch, func() bool { return srv.accepted.Load() == 1 })
	cancel()
	res := wait(t, "Check after cancel", ch)

	if res.Status != certcheck.Error {
		t.Errorf("Status = %v, want ERROR", res.Status)
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Errorf("Err = %v, want an error that matches context.Canceled with errors.Is", res.Err)
	}
}

// One Checker is shared: CheckAll's workers use it at the same time, and so
// can the caller's own goroutines. So Check must not write to it, not even to
// fill in a default. (go test -race also reports such a write as a data race.)
func TestCheckSharedChecker(t *testing.T) {
	f := pki()
	srv := serveChain(t, f.www, f.inter)
	tg := certcheck.Target{Name: "www", Addr: srv.addr, ServerName: "www.example.com"}
	// Only Roots is set: Timeout 0 means DefaultTimeout, Now nil means the
	// real clock, the thresholds are off.
	c := &certcheck.Checker{Roots: f.roots}

	var running []<-chan outcome[certcheck.Result]
	for range 4 {
		running = append(running, startCheck(t.Context(), c, tg))
	}
	for _, ch := range running {
		res := wait(t, "Check", ch)
		// The real clock is long past the leaf's NotAfter (2025-04-30).
		if res.Status != certcheck.Expired {
			t.Errorf("Status = %v, want EXPIRED: with Now nil, Check uses the real clock (Err: %s)", res.Status, errText(res.Err))
		}
	}
	if c.Timeout != 0 {
		t.Errorf("after Check, Checker.Timeout = %v, want 0: Check must not modify the Checker", c.Timeout)
	}
	if c.Now != nil {
		t.Errorf("after Check, Checker.Now is set, want nil: Check must not modify the Checker")
	}
}

// A checker that runs every minute must not leak a connection per check.
func TestCheckClosesConnection(t *testing.T) {
	f := pki()
	srv := serveChain(t, f.www, f.inter)
	tg := certcheck.Target{Name: "www", Addr: srv.addr, ServerName: "www.example.com"}

	res := runCheck(t, t.Context(), newChecker(base), tg)
	if res.Status != certcheck.OK {
		t.Fatalf("Status = %v, want OK (Err: %s)", res.Status, errText(res.Err))
	}
	// The server sees the end of the connection only when the client closes it.
	waitFor(t, "the connection is closed after Check returned", func() bool { return srv.ended.Load() == 1 })
	if n := srv.accepted.Load(); n != 1 {
		t.Errorf("the server accepted %d connections, want 1", n)
	}
}
