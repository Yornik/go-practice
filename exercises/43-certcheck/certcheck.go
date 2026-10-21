// Package certcheck connects to TLS endpoints, verifies the certificate chain
// each one serves and reports how long it is until a certificate in that
// chain expires.
package certcheck

import (
	"context"
	"crypto/x509"
	"fmt"
	"time"
)

// DefaultTimeout is the time limit of one check when Checker.Timeout is zero
// or negative.
const DefaultTimeout = 10 * time.Second

// Target is one TLS endpoint to check.
type Target struct {
	// Name is a label for the report, for example "www" or "api-eu".
	Name string
	// Addr is the address to connect to, as host:port, for example
	// "www.example.com:443" or "10.0.12.7:443".
	Addr string
	// ServerName is the host name to send in the handshake (SNI) and to
	// verify the certificate for. Empty means the host part of Addr.
	ServerName string
}

// Status is the verdict of one check. The constants are ordered from good to
// bad.
type Status int

const (
	// OK: the chain verifies and no certificate expires within Warn.
	OK Status = iota
	// Warning: the chain verifies, a certificate expires within Warn.
	Warning
	// Critical: the chain verifies, a certificate expires within Critical.
	Critical
	// Expired: a certificate the server sent has expired.
	Expired
	// Invalid: the chain does not verify for a reason other than expiry.
	Invalid
	// Error: no certificate to look at (bad address, connect or handshake
	// failure, timeout, cancelled context).
	Error
)

var statusNames = [...]string{"OK", "WARNING", "CRITICAL", "EXPIRED", "INVALID", "ERROR"}

// String returns the upper-case name of the status, for example "WARNING",
// or "Status(N)" for an unknown value.
func (s Status) String() string {
	if s < 0 || int(s) >= len(statusNames) {
		return fmt.Sprintf("Status(%d)", int(s))
	}
	return statusNames[s]
}

// Result is the outcome of checking one Target.
type Result struct {
	// Target is the target that was checked, exactly as it was passed in.
	Target Target
	// Status is the verdict.
	Status Status
	// NotAfter is the earliest NotAfter of all certificates the server sent.
	// Zero when Status is Error.
	NotAfter time.Time
	// Remaining is NotAfter minus the check time; negative once expired.
	// Zero when Status is Error.
	Remaining time.Duration
	// Subject and Issuer of the leaf certificate, as returned by
	// pkix.Name.String, for example "CN=www.example.com". Empty when Status
	// is Error.
	Subject, Issuer string
	// DNSNames of the leaf certificate. Nil when Status is Error.
	DNSNames []string
	// Err is the connect or handshake error when Status is Error, otherwise
	// the error from verifying the chain (nil if it verified).
	Err error
}

// Checker checks TLS endpoints. The zero value is usable: system roots, the
// default timeout, the real clock, no warning thresholds, one check at a
// time. A Checker must not be modified while checks are running; Check and
// CheckAll never modify it and are safe for concurrent use.
type Checker struct {
	// Roots are the trusted root certificates. Nil means the system roots.
	Roots *x509.CertPool
	// Timeout is the time limit of one check: connect plus handshake.
	// Zero or negative means DefaultTimeout.
	Timeout time.Duration
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time
	// Warn and Critical are the thresholds for Warning and Critical: the
	// status is Critical if Remaining < Critical, else Warning if
	// Remaining < Warn. Zero disables a threshold.
	Warn, Critical time.Duration
	// Concurrency is the maximum number of checks CheckAll runs at the same
	// time. Values below 1 mean 1.
	Concurrency int
}

// Check connects to t.Addr, performs a TLS handshake, verifies the chain the
// server sent and classifies the result (see Status and the README). It
// reports the expiry of the chain even when the chain does not verify.
//
// Check opens one connection and closes it before it returns. It gives up
// when Timeout passes or ctx is done, whichever comes first. If ctx is done,
// Err matches ctx.Err() with errors.Is.
func (c *Checker) Check(ctx context.Context, t Target) Result {
	panic("TODO: implement")
}

// CheckAll checks every target, at most Concurrency at the same time, and
// returns one Result per target, in the order of ts. A new check starts as
// soon as a running one finishes, so one slow target holds up only its own
// slot. When ctx is done, CheckAll opens no new connections: every target
// that was not checked yet gets a Result with its Target, Status Error and an
// Err that matches ctx.Err() with errors.Is.
func (c *Checker) CheckAll(ctx context.Context, ts []Target) []Result {
	panic("TODO: implement")
}
