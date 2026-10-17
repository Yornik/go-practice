package certcheck_test

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	certcheck "github.com/yornik/go-practice/exercises/18-certcheck"
)

// These tests build Results by hand; they do not connect anywhere.

// result is a Result as Check would return it, Remaining after base.
func result(name string, s certcheck.Status, remaining time.Duration, err error) certcheck.Result {
	return certcheck.Result{
		Target:    certcheck.Target{Name: name, Addr: name + ".example.com:443"},
		Status:    s,
		NotAfter:  base.Add(remaining),
		Remaining: remaining,
		Err:       err,
	}
}

// failed is a Result with Status Error: no certificate, so no NotAfter.
func failed(name string, err error) certcheck.Result {
	return certcheck.Result{
		Target: certcheck.Target{Name: name, Addr: name + ".example.com:443"},
		Status: certcheck.Error,
		Err:    err,
	}
}

// report calls WriteReport and returns what it wrote. It fails the test if
// WriteReport panics or returns an error.
func report(t *testing.T, rs []certcheck.Result) string {
	t.Helper()
	var buf bytes.Buffer
	err := wait(t, "WriteReport", start(func() error { return certcheck.WriteReport(&buf, rs) }))
	if err != nil {
		t.Fatalf("WriteReport() error = %v, want nil", err)
	}
	return buf.String()
}

// diffLines shows got and want line by line, so that a wrong space is easy
// to find. Dots mark the spaces.
func diffLines(got, want string) string {
	var b strings.Builder
	b.WriteString("\n--- got (spaces shown as dots)\n")
	b.WriteString(strings.ReplaceAll(got, " ", "."))
	b.WriteString("\n--- want\n")
	b.WriteString(strings.ReplaceAll(want, " ", "."))
	return b.String()
}

func TestReportFormat(t *testing.T) {
	cet := time.FixedZone("CET", 3600)
	api := result("api", certcheck.Warning, 19*day, nil)
	api.NotAfter = api.NotAfter.In(cet) // the same instant; the report prints UTC

	rs := []certcheck.Result{
		result("www", certcheck.OK, 89*day+23*time.Hour, nil),
		api,
		failed("legacy", errors.New("dial tcp 10.0.0.9:443: connect: connection refused")),
		result("shop", certcheck.Critical, 2*day+6*time.Hour, nil),
		result("portal", certcheck.Expired, -(2*day + 5*time.Hour), errors.New("x509: certificate has expired or is not yet valid")),
		result("intranet", certcheck.Invalid, 76*day, errors.New("x509: certificate signed by unknown authority")),
		result("cdn", certcheck.OK, 89*day+23*time.Hour, nil),
	}
	want := `STATUS    NAME      EXPIRES               DAYS  DETAIL
ERROR     legacy    -                     -     dial tcp 10.0.0.9:443: connect: connection refused
INVALID   intranet  2025-05-16T12:00:00Z  76    x509: certificate signed by unknown authority
EXPIRED   portal    2025-02-27T07:00:00Z  -2    x509: certificate has expired or is not yet valid
CRITICAL  shop      2025-03-03T18:00:00Z  2     -
WARNING   api       2025-03-20T12:00:00Z  19    -
OK        cdn       2025-05-30T11:00:00Z  89    -
OK        www       2025-05-30T11:00:00Z  89    -
`
	if got := report(t, rs); got != want {
		t.Errorf("WriteReport wrote the wrong table:%s", diffLines(got, want))
	}
}

// Within one status: Remaining ascending, then Name. Rows that are equal in
// all three keep their input order.
func TestReportOrder(t *testing.T) {
	rs := []certcheck.Result{
		result("b", certcheck.OK, 50*day, nil),
		failed("dns", errors.New("second")),
		result("a", certcheck.OK, 50*day, nil),
		result("z", certcheck.OK, 40*day, nil),
		result("gone", certcheck.Expired, -1*day, nil),
		failed("dns", errors.New("third")),
		result("long-gone", certcheck.Expired, -5*day, nil),
		failed("api", errors.New("first")),
	}
	want := `STATUS   NAME       EXPIRES               DAYS  DETAIL
ERROR    api        -                     -     first
ERROR    dns        -                     -     second
ERROR    dns        -                     -     third
EXPIRED  long-gone  2025-02-24T12:00:00Z  -5    -
EXPIRED  gone       2025-02-28T12:00:00Z  -1    -
OK       z          2025-04-10T12:00:00Z  40    -
OK       a          2025-04-20T12:00:00Z  50    -
OK       b          2025-04-20T12:00:00Z  50    -
`
	if got := report(t, rs); got != want {
		t.Errorf("WriteReport wrote the rows in the wrong order:%s", diffLines(got, want))
	}
}

// A name can appear more than once, for example with one target per IP
// address of a host name (see the stretch goals). Here "edge" has three
// addresses and all three fail, so their rows are equal in status, Remaining
// and name: they keep their input order. The report has 13 rows, one per
// domain of the scenario. Some sort algorithms keep equal rows in order in
// short slices by chance, so the short tests above cannot show the difference.
func TestReportStableOrder(t *testing.T) {
	refused := func(ip string) error {
		return errors.New("dial tcp " + ip + ":443: connect: connection refused")
	}
	rs := []certcheck.Result{
		result("www", certcheck.OK, 80*day, nil),
		failed("edge", refused("10.0.0.1")),
		result("api", certcheck.Warning, 20*day, nil),
		result("cdn", certcheck.OK, 60*day, nil),
		result("docs", certcheck.OK, 70*day, nil),
		failed("edge", refused("10.0.0.2")),
		result("status", certcheck.OK, 85*day, nil),
		result("shop", certcheck.Critical, 3*day, nil),
		result("blog", certcheck.OK, 50*day, nil),
		failed("edge", refused("10.0.0.3")),
		result("mail", certcheck.OK, 40*day, nil),
		result("auth", certcheck.OK, 75*day, nil),
		result("img", certcheck.OK, 65*day, nil),
	}
	want := `STATUS    NAME    EXPIRES               DAYS  DETAIL
ERROR     edge    -                     -     dial tcp 10.0.0.1:443: connect: connection refused
ERROR     edge    -                     -     dial tcp 10.0.0.2:443: connect: connection refused
ERROR     edge    -                     -     dial tcp 10.0.0.3:443: connect: connection refused
CRITICAL  shop    2025-03-04T12:00:00Z  3     -
WARNING   api     2025-03-21T12:00:00Z  20    -
OK        mail    2025-04-10T12:00:00Z  40    -
OK        blog    2025-04-20T12:00:00Z  50    -
OK        cdn     2025-04-30T12:00:00Z  60    -
OK        img     2025-05-05T12:00:00Z  65    -
OK        docs    2025-05-10T12:00:00Z  70    -
OK        auth    2025-05-15T12:00:00Z  75    -
OK        www     2025-05-20T12:00:00Z  80    -
OK        status  2025-05-25T12:00:00Z  85    -
`
	if got := report(t, rs); got != want {
		t.Errorf("WriteReport wrote the rows in the wrong order (the three edge rows keep their input order):%s", diffLines(got, want))
	}
}

// The caller's slice stays in the caller's order.
func TestReportDoesNotReorderInput(t *testing.T) {
	rs := []certcheck.Result{
		result("www", certcheck.OK, 80*day, nil),
		failed("legacy", errors.New("connection refused")),
		result("api", certcheck.Warning, 10*day, nil),
	}
	before := slices.Clone(rs)
	report(t, rs)

	for i := range rs {
		if rs[i].Target.Name != before[i].Target.Name {
			t.Fatalf("after WriteReport, rs[%d] is %q, want %q: WriteReport must not reorder its input", i, rs[i].Target.Name, before[i].Target.Name)
		}
	}
}

func TestReportNoResults(t *testing.T) {
	want := "STATUS  NAME  EXPIRES  DAYS  DETAIL\n"
	if got := report(t, nil); got != want {
		t.Errorf("WriteReport(no results) wrote the wrong table:%s", diffLines(got, want))
	}
}

var errDiskFull = errors.New("no space left on device")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errDiskFull }

func TestReportWriteError(t *testing.T) {
	rs := []certcheck.Result{result("www", certcheck.OK, 80*day, nil)}
	var w io.Writer = failingWriter{}

	err := wait(t, "WriteReport", start(func() error { return certcheck.WriteReport(w, rs) }))

	if !errors.Is(err, errDiskFull) {
		t.Errorf("WriteReport(failing writer) = %v, want an error that matches the writer's error with errors.Is", err)
	}
}
