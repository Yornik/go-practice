package certcheck_test

// Test fixtures: a small PKI (public key infrastructure) and servers on
// 127.0.0.1.
//
// The tests build their own certificate authority with crypto/x509, the way a
// real one is built: a self-signed root, an intermediate signed by the root,
// and leaf certificates signed by the intermediate. The servers send the leaf
// and the intermediate, as real servers do. The root is in the Checker's
// Roots pool, as it would be in the system trust store.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	certcheck "github.com/yornik/go-practice/exercises/43-certcheck"
)

// base is the "now" of the tests: Checker.Now returns base or a time
// relative to it, and the certificates are built around it. It lies in the
// past on purpose. A solution that reads the real clock instead of calling
// Checker.Now sees every leaf certificate as expired.
var base = time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)

const day = 24 * time.Hour

// limit bounds every wait in these tests, so that a wrong solution fails with
// a message instead of hanging.
const limit = 10 * time.Second

// window is how long a test watches for something that must NOT happen, such
// as a 4th check starting while the limit is 3. A correct solution never
// does it, so the window only decides how likely a wrong solution is caught.
const window = 100 * time.Millisecond

// fixtures holds the certificates the tests share. They are built once.
type fixtures struct {
	roots      *x509.CertPool // trusts "Test Root CA" only
	otherRoots *x509.CertPool // trusts an unrelated root only

	root     *x509.Certificate // Test Root CA: base-10y .. base+10y
	inter    *x509.Certificate // Test Intermediate CA, signed by root: base-1y .. base+3y
	interKey *ecdsa.PrivateKey
	leafKey  *ecdsa.PrivateKey // the key of every leaf certificate

	// www is the main leaf: www.example.com, example.com and 127.0.0.1,
	// signed by inter, valid from base-30d to base+60d.
	www *x509.Certificate

	// oldInter is an intermediate that expired at base-2d. old is a leaf
	// for old.example.com signed by it, itself valid until base+60d.
	oldInter *x509.Certificate
	old      *x509.Certificate

	// cross is a second certificate for Test Root CA (same name, same key),
	// signed by a legacy root, which expired at base-10d. Servers sometimes
	// still send such a cross-signed certificate after it expired (the
	// AddTrust expiry of 2020).
	cross *x509.Certificate
}

var serial atomic.Int64

func newKey() *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return key
}

// sign creates the certificate described by tmpl for the public key pub,
// signed by parent with parentKey. For a self-signed certificate, parent is
// tmpl itself.
func sign(tmpl, parent *x509.Certificate, pub *ecdsa.PublicKey, parentKey *ecdsa.PrivateKey) *x509.Certificate {
	tmpl.SerialNumber = big.NewInt(serial.Add(1))
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, parentKey)
	if err != nil {
		panic(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	return cert
}

func caTemplate(name string, notBefore, notAfter time.Time) *x509.Certificate {
	return &x509.Certificate{
		Subject:               pkix.Name{CommonName: name, Organization: []string{"go-practice"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
}

// leafTemplate describes a server certificate for names (host names or IP
// addresses). The first name is the subject's common name.
func leafTemplate(names []string, notBefore, notAfter time.Time) *x509.Certificate {
	tmpl := &x509.Certificate{
		Subject:     pkix.Name{CommonName: names[0]},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, n := range names {
		if ip := net.ParseIP(n); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, n)
		}
	}
	return tmpl
}

var pki = sync.OnceValue(func() *fixtures {
	f := &fixtures{leafKey: newKey()}

	rootKey := newKey()
	rootTmpl := caTemplate("Test Root CA", base.Add(-3650*day), base.Add(3650*day))
	f.root = sign(rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	f.roots = x509.NewCertPool()
	f.roots.AddCert(f.root)

	f.interKey = newKey()
	f.inter = sign(caTemplate("Test Intermediate CA", base.Add(-365*day), base.Add(3*365*day)),
		f.root, &f.interKey.PublicKey, rootKey)

	f.www = f.leaf([]string{"www.example.com", "example.com", "127.0.0.1"}, base.Add(-30*day), base.Add(60*day))

	oldKey := newKey()
	f.oldInter = sign(caTemplate("Old Intermediate CA", base.Add(-800*day), base.Add(-2*day)),
		f.root, &oldKey.PublicKey, rootKey)
	f.old = sign(leafTemplate([]string{"old.example.com"}, base.Add(-30*day), base.Add(60*day)),
		f.oldInter, &f.leafKey.PublicKey, oldKey)

	legacyKey := newKey()
	legacyTmpl := caTemplate("Legacy Root CA", base.Add(-7000*day), base.Add(-10*day))
	legacy := sign(legacyTmpl, legacyTmpl, &legacyKey.PublicKey, legacyKey)
	f.cross = sign(caTemplate("Test Root CA", base.Add(-2000*day), base.Add(-10*day)),
		legacy, &rootKey.PublicKey, legacyKey)

	otherKey := newKey()
	otherTmpl := caTemplate("Other Root CA", base.Add(-3650*day), base.Add(3650*day))
	f.otherRoots = x509.NewCertPool()
	f.otherRoots.AddCert(sign(otherTmpl, otherTmpl, &otherKey.PublicKey, otherKey))
	return f
})

// leaf issues a leaf certificate for names, signed by the intermediate.
func (f *fixtures) leaf(names []string, notBefore, notAfter time.Time) *x509.Certificate {
	return sign(leafTemplate(names, notBefore, notAfter), f.inter, &f.leafKey.PublicKey, f.interKey)
}

// tlsCert turns a chain (leaf first) into what a tls.Config serves.
func tlsCert(chain ...*x509.Certificate) *tls.Certificate {
	c := &tls.Certificate{PrivateKey: pki().leafKey}
	for _, cert := range chain {
		c.Certificate = append(c.Certificate, cert.Raw)
	}
	return c
}

// server is a TCP server on 127.0.0.1 for one test. It is shut down when the
// test ends.
type server struct {
	addr     string
	accepted atomic.Int32 // connections accepted so far
	ended    atomic.Int32 // connections that are over (the client closed it, or it failed)
}

// startServer runs handle for every accepted connection. ctx is cancelled
// when the test ends; then all connections are closed and the server waits
// for every handle to return.
func startServer(t *testing.T, handle func(ctx context.Context, c net.Conn)) *server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &server{addr: ln.Addr().String()}
	ctx, cancel := context.WithCancel(context.Background())

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		conns    = map[net.Conn]bool{}
		stopping bool
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.accepted.Add(1)
			mu.Lock()
			if stopping {
				mu.Unlock()
				_ = c.Close()
				return
			}
			conns[c] = true
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				handle(ctx, c)
				_ = c.Close()
				s.ended.Add(1)
			}()
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = ln.Close()
		mu.Lock()
		stopping = true
		for c := range conns {
			_ = c.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return s
}

// startTLS runs a TLS server with cfg. After the handshake it reads until
// the client closes the connection.
func startTLS(t *testing.T, cfg *tls.Config) *server {
	t.Helper()
	return startServer(t, func(ctx context.Context, c net.Conn) {
		tc := tls.Server(c, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, tc)
	})
}

// serveChain runs a TLS server that sends chain (leaf first) to every client.
func serveChain(t *testing.T, chain ...*x509.Certificate) *server {
	t.Helper()
	return startTLS(t, &tls.Config{Certificates: []tls.Certificate{*tlsCert(chain...)}})
}

// startSilent runs a server that accepts TCP connections and then never
// answers: it reads what the client sends and writes nothing. A load
// balancer without healthy backends, or an overloaded server, looks like
// this from the outside.
func startSilent(t *testing.T) *server {
	t.Helper()
	return startServer(t, func(_ context.Context, c net.Conn) {
		_, _ = io.Copy(io.Discard, c)
	})
}

// startSSH runs a server that is not a TLS server at all: it greets every
// client with an SSH banner, as port 22 would.
func startSSH(t *testing.T) *server {
	t.Helper()
	return startServer(t, func(_ context.Context, c net.Conn) {
		_, _ = io.WriteString(c, "SSH-2.0-OpenSSH_9.6\r\n")
		_, _ = io.Copy(io.Discard, c)
	})
}

// closedAddr returns an address on 127.0.0.1 where nothing listens.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

// newChecker returns a Checker that trusts the test root, warns at 30 days,
// is critical at 7 days, and whose clock always says now.
func newChecker(now time.Time) *certcheck.Checker {
	return &certcheck.Checker{
		Roots:    pki().roots,
		Timeout:  5 * time.Second,
		Now:      func() time.Time { return now },
		Warn:     30 * day,
		Critical: 7 * day,
	}
}

// runCheck calls c.Check in a new goroutine and waits for it. It fails the
// test if Check panics or does not return within the limit.
func runCheck(t *testing.T, ctx context.Context, c *certcheck.Checker, tg certcheck.Target) certcheck.Result {
	t.Helper()
	return wait(t, "Check", startCheck(ctx, c, tg))
}

type outcome[T any] struct {
	val      T
	panicked any
}

// startCheck calls c.Check in a new goroutine. The channel receives the
// result when Check returns.
func startCheck(ctx context.Context, c *certcheck.Checker, tg certcheck.Target) <-chan outcome[certcheck.Result] {
	return start(func() certcheck.Result { return c.Check(ctx, tg) })
}

// startCheckAll calls c.CheckAll in a new goroutine. The channel receives the
// results when CheckAll returns.
func startCheckAll(ctx context.Context, c *certcheck.Checker, ts []certcheck.Target) <-chan outcome[[]certcheck.Result] {
	return start(func() []certcheck.Result { return c.CheckAll(ctx, ts) })
}

func start[T any](f func() T) <-chan outcome[T] {
	ch := make(chan outcome[T], 1)
	go func() {
		var o outcome[T]
		defer func() {
			o.panicked = recover()
			ch <- o
		}()
		o.val = f()
	}()
	return ch
}

// wait waits for what start delivers. It fails the test if the function
// panicked or did not return within the limit.
func wait[T any](t *testing.T, what string, ch <-chan outcome[T]) T {
	t.Helper()
	select {
	case o := <-ch:
		if o.panicked != nil {
			t.Fatalf("%s panicked: %v", what, o.panicked)
		}
		return o.val
	case <-time.After(limit):
		t.Fatalf("%s did not return within %v", what, limit)
	}
	var zero T
	return zero
}

// waitFor polls cond until it returns true. It fails the test if that takes
// longer than the limit.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting until %s", limit, what)
		}
		time.Sleep(time.Millisecond)
	}
}

// waitWhileRunning polls cond like waitFor, while the function behind ch is
// still running. If that function returns or panics before cond is true,
// the test fails at once and shows what happened: the function should still
// have been busy.
func waitWhileRunning[T any](t *testing.T, fn, what string, ch <-chan outcome[T], cond func() bool) {
	t.Helper()
	met := false
	waitFor(t, what, func() bool {
		met = cond()
		return met || len(ch) > 0
	})
	if met {
		return
	}
	o := <-ch // does not block: len(ch) > 0
	if o.panicked != nil {
		t.Fatalf("%s panicked: %v", fn, o.panicked)
	}
	t.Fatalf("%s returned before %s: %+v", fn, what, o.val)
}

// errText returns err.Error(), or "<nil>", for failure messages.
func errText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
