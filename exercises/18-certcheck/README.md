# 18 · TLS certificate expiry checker

**Tier 4 · about 3 h** · Go: `crypto/tls` client config and `tls.Dialer`, `crypto/x509` chain verification and its error types, `errors.As` and `errors.Is`, timeouts and cancellation on network calls, bounded concurrency, `text/tabwriter` · Problem solving: classify failures so an operator knows what to do; read test fixtures that build their own PKI

You run 13 public HTTPS domains behind ingress controllers and a CDN. cert-manager renews the
certificates, until the day it silently stops: rotated DNS credentials, a rate limit, a broken
webhook. Let's Encrypt stopped sending expiry reminder emails in 2025, so nobody warns you any more.
And the leaf is not the only certificate that can expire: intermediates expire too, and a server
can send a chain that your browser accepts while `curl` and Go clients reject it. You want a
small tool for a cron job: connect to every endpoint the way a client does, verify the chain and
the host name, and print a table with the most urgent problem on top, weeks before a customer
sees a certificate error.

## Task

Implement the stubs in `certcheck.go` and `report.go`. `Status` (with `String`) and
`DefaultTimeout` are given.

```go
type Target struct {
	Name       string // label for the report
	Addr       string // host:port to connect to
	ServerName string // name to send (SNI) and to verify; "" means the host of Addr
}

type Status int // OK, Warning, Critical, Expired, Invalid, Error

type Result struct {
	Target          Target        // the target exactly as passed in
	Status          Status
	NotAfter        time.Time     // earliest NotAfter of all certificates the server sent
	Remaining       time.Duration // NotAfter - Now()
	Subject, Issuer string        // of the leaf, as pkix.Name.String returns them
	DNSNames        []string      // of the leaf
	Err             error
}

type Checker struct {
	Roots          *x509.CertPool   // trusted roots; nil means the system roots
	Timeout        time.Duration    // per check, connect + handshake; <= 0 means DefaultTimeout (10s)
	Now            func() time.Time // nil means time.Now
	Warn, Critical time.Duration    // thresholds; 0 disables one
	Concurrency    int              // for CheckAll; < 1 means 1
}

func (c *Checker) Check(ctx context.Context, t Target) Result      // one endpoint
func (c *Checker) CheckAll(ctx context.Context, ts []Target) []Result // many, results in input order
func WriteReport(w io.Writer, rs []Result) error                    // a sorted text table
```

The core problem: a TLS client with the default configuration aborts the handshake when the chain
does not verify, and "certificate signed by unknown authority" says nothing about when anything
expires. Find a way to get the chain the server sends even when it is bad, and verify it yourself
with `crypto/x509`.

## Rules

### Check: the connection

- `ServerName` defaults to the host part of `Addr` (`"127.0.0.1"` for `"127.0.0.1:8443"`).
  Check sends it in the handshake (SNI): a server with many certificates picks one by that name.
  (Go never sends an IP address as SNI; that is fine.)
- `Timeout` covers connect and handshake together. A server that accepts the TCP connection and
  then never answers must not hold Check longer than `Timeout`.
- Check stops as soon as `ctx` is done. `Err` then matches `ctx.Err()` with `errors.Is`.
- Check opens one connection, sends no data on it, and closes it before it returns.
- `Result.Target` is `t` exactly as passed in. An empty `ServerName` stays empty.
- Check takes the time from `Now`, once per check, not from `time.Now` directly. The tests use a
  clock in the past.
- Check and CheckAll never modify the Checker, not even to fill in a default: one Checker is
  used by many goroutines at the same time.

### Check: the certificate fields

- `NotAfter` is the earliest `NotAfter` of **all** certificates the server sent: the leaf, the
  intermediates, and anything extra. `Remaining = NotAfter - Now()`, negative once expired.
- `Subject` and `Issuer` are the leaf's `Subject.String()` and `Issuer.String()`, for example
  `CN=www.example.com` and `CN=Test Intermediate CA,O=go-practice`. `DNSNames` is the leaf's
  `DNSNames`.
- These fields are filled whenever the server sent a certificate, also when the chain does not
  verify. With Status ERROR they are all zero.
- Verify the chain as a client would: the leaf must chain up to a certificate in `Roots` through
  the intermediates the server sent; it must be valid for `ServerName` (a DNS name or an IP
  address); everything at the time `Now()`.
- `Err` is the error from verification, as is or wrapped, so that `errors.As` finds
  `x509.UnknownAuthorityError`, `x509.HostnameError` or `x509.CertificateInvalidError`. It is nil
  if the chain verified.

### Check: the status (first matching row wins)

| Status | When | Err |
|---|---|---|
| ERROR | no certificate: `Addr` without port, connect fails, timeout, `ctx` done, handshake fails (for example: not a TLS server) | that error |
| EXPIRED | `Now()` is after `NotAfter` | the verification error, or nil if the chain verified anyway |
| INVALID | verification failed: unknown authority, intermediate missing, host name mismatch, not valid yet | the verification error |
| CRITICAL | `Remaining < Critical` | nil |
| WARNING | `Remaining < Warn` | nil |
| OK | otherwise | nil |

- A certificate is valid up to and including its `NotAfter`. At `Now() == NotAfter` it has not
  expired (Remaining 0: CRITICAL). One second later it is EXPIRED.
- The thresholds are strict: `Remaining == Warn` is OK, `Remaining == Critical` is WARNING. A
  threshold of 0 never fires.
- EXPIRED is decided by the clock and `NotAfter`, not by the kind of error verification returns.
  An expired intermediate is EXPIRED, with the intermediate's `NotAfter`. An expired extra
  certificate that the chain does not need (an old cross-signed root, as in the AddTrust expiry of
  2020) does not fail verification, but it is still EXPIRED, with `Err` nil.
- A certificate that is not valid yet (`NotBefore` after `Now()`) is INVALID. `x509` reports it
  with `Reason == x509.Expired`, but nothing has expired.

### CheckAll

- One Result per target: `results[i]` belongs to `ts[i]`, whatever order the checks finish in. No
  targets: an empty result.
- At most `Concurrency` checks run at the same time (`< 1` means 1). With enough targets, CheckAll
  uses all of them.
- No batches: a new check starts as soon as any running check finishes. One slow target holds up
  only its own slot.
- When `ctx` is done: the running checks stop (see Check), CheckAll opens no new connections, and
  every target that was not checked gets a Result with its `Target`, Status ERROR and an `Err`
  that matches `ctx.Err()`. CheckAll still returns one Result per target.

### WriteReport

- A header line, then one line per result. Every line ends with `\n`. The columns are `STATUS`,
  `NAME`, `EXPIRES`, `DAYS`, `DETAIL`.
- STATUS is `Status.String()`. NAME is `Target.Name`. EXPIRES is `NotAfter` in UTC, in RFC 3339
  (`2025-05-16T12:00:00Z`). DAYS is `Remaining` in whole days, truncated toward zero
  (89d23h is `89`, -2d5h is `-2`, -12h is `0`). DETAIL is `Err.Error()`, or `-` if `Err` is nil.
  For ERROR rows, EXPIRES and DAYS are `-`.
- Layout: columns are left-aligned. Every column except the last is padded with spaces to the
  width of its widest cell (header included) plus 2. The last column is not padded, so no line
  ends with a space.
- Order: most severe status first (ERROR, INVALID, EXPIRED, CRITICAL, WARNING, OK: the reverse of
  the constant order), then `Remaining` ascending, then `Name`. Rows equal in all three (one name
  checked on several addresses) keep their input order, however many rows there are.
- WriteReport does not reorder `rs`.
- No results: only the header line.
- It returns the first error from `w`, so that `errors.Is` finds it.

## Examples

```go
c := &certcheck.Checker{
	Timeout:     5 * time.Second,
	Warn:        30 * 24 * time.Hour,
	Critical:    7 * 24 * time.Hour,
	Concurrency: 4,
}
res := c.Check(ctx, certcheck.Target{Name: "api", Addr: "10.0.12.7:443", ServerName: "api.example.com"})
```

Results for servers from the tests. The leaf is valid until 2025-04-30 and `Now()` is
2025-03-01, unless the row says otherwise:

```
server sends                                 Status   NotAfter    Err
leaf + intermediate                          OK       2025-04-30  <nil>
the same, Now() = 2025-04-10                 WARNING  2025-04-30  <nil>   (20 days left)
the same, ServerName shop.example.com        INVALID  2025-04-30  x509: certificate is valid for www.example.com, example.com, not shop.example.com
leaf only (intermediate missing)             INVALID  2025-04-30  x509: certificate signed by unknown authority
leaf + intermediate that expired 2025-02-27  EXPIRED  2025-02-27  x509: certificate has expired or is not yet valid: ...
leaf + intermediate + expired cross-sign     EXPIRED  2025-02-19  <nil>
an SSH banner                                ERROR    (zero)      tls: first record does not look like a TLS handshake
```

A report (`Now()` = 2025-03-01T12:00:00Z):

```
STATUS    NAME      EXPIRES               DAYS  DETAIL
ERROR     legacy    -                     -     dial tcp 10.0.0.9:443: connect: connection refused
INVALID   intranet  2025-05-16T12:00:00Z  76    x509: certificate signed by unknown authority
EXPIRED   portal    2025-02-27T07:00:00Z  -2    x509: certificate has expired or is not yet valid
CRITICAL  shop      2025-03-03T18:00:00Z  2     -
WARNING   api       2025-03-20T12:00:00Z  19    -
OK        cdn       2025-05-30T11:00:00Z  89    -
OK        www       2025-05-30T11:00:00Z  89    -
```

## Run the tests

    go test ./exercises/18-certcheck/
    go test -race ./exercises/18-certcheck/
    go test -run TestCheck -skip TestCheckAll -v ./exercises/18-certcheck/   # step 1: Check
    go test -run TestCheckAll -v ./exercises/18-certcheck/                   # step 2: CheckAll
    go test -run TestReport -v ./exercises/18-certcheck/                     # step 3: WriteReport
    go test -run TestCheckInvalid -v ./exercises/18-certcheck/               # one test

WriteReport does not depend on the other two; you can start with it if you want a warm-up.

The tests build their own certificate authority (root, intermediate, leaves) with `crypto/x509`
and serve the chains from TLS servers on 127.0.0.1; read `helpers_test.go` to see how. Nothing
touches the internet, and the package runs in about a second. If your code blocks, a test fails
with "timed out after 10s waiting until ..." or "did not return within 10s".

## Docs you can use

- [`crypto/tls.Config`](https://pkg.go.dev/crypto/tls#Config): read the fields `ServerName`,
  `InsecureSkipVerify`, `VerifyConnection` and their comments
- [`crypto/tls.Dialer`](https://pkg.go.dev/crypto/tls#Dialer),
  [`tls.Conn.ConnectionState`](https://pkg.go.dev/crypto/tls#Conn.ConnectionState),
  [`tls.ConnectionState`](https://pkg.go.dev/crypto/tls#ConnectionState)
- [`x509.Certificate.Verify`](https://pkg.go.dev/crypto/x509#Certificate.Verify),
  [`x509.VerifyOptions`](https://pkg.go.dev/crypto/x509#VerifyOptions),
  [`x509.CertPool`](https://pkg.go.dev/crypto/x509#CertPool), and the error types
  [`HostnameError`](https://pkg.go.dev/crypto/x509#HostnameError),
  [`UnknownAuthorityError`](https://pkg.go.dev/crypto/x509#UnknownAuthorityError),
  [`CertificateInvalidError`](https://pkg.go.dev/crypto/x509#CertificateInvalidError)
- [`pkix.Name.String`](https://pkg.go.dev/crypto/x509/pkix#Name.String)
- [`net.SplitHostPort`](https://pkg.go.dev/net#SplitHostPort), [`net.Dialer`](https://pkg.go.dev/net#Dialer)
- [`context.WithTimeout`](https://pkg.go.dev/context#WithTimeout)
- [`errors.As`](https://pkg.go.dev/errors#As), [`errors.Is`](https://pkg.go.dev/errors#Is)
- [`text/tabwriter`](https://pkg.go.dev/text/tabwriter)
- [`slices.SortStableFunc`](https://pkg.go.dev/slices#SortStableFunc), [`cmp.Or`](https://pkg.go.dev/cmp#Or)
- [`time.Time.UTC`](https://pkg.go.dev/time#Time.UTC), [`time.RFC3339`](https://pkg.go.dev/time#pkg-constants)
- 12-workerpool, for bounded concurrency with results in input order

## Stretch goals

- A small `main` package that reads targets from a file, prints the report and exits with
  status 2 if any row is ERROR, INVALID or EXPIRED, 1 for CRITICAL or WARNING, else 0, so that a
  cron job or CI step can alert on it.
- A `/metrics` endpoint with one gauge per target, `cert_not_after_timestamp_seconds{name="www"}`,
  and alert in Prometheus with `cert_not_after_timestamp_seconds - time() < 14 * 86400`.
- Resolve every A and AAAA record of a name and check each IP address: with DNS round robin, one
  old node can serve an old certificate to a fraction of your users.
- Say which certificate expires first: add the subject of the earliest certificate to the Result
  and to DETAIL.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 18 hint 1
    go run ./cmd/peek 18 hint 2
    go run ./cmd/peek 18 hint 3
    go run ./cmd/peek 18 explanation
    go run ./cmd/peek 18 walkthrough
    go run ./cmd/peek 18 solution
