package tracetree_test

import (
	"fmt"
	"strings"
	"testing"

	tracetree "github.com/yornik/go-practice/exercises/36-tracetree"
)

// named returns a span like span() does, with the given Name.
func named(id, parent, service, name string, start, end int) tracetree.Span {
	s := span(id, parent, service, start, end)
	s.Name = name
	return s
}

// The Render tests use Build, SelfTime and CriticalPath: do them first.
// Lines on the critical path start with "*". Expected output is compared
// exactly, including the "\n" at the end of the last line.
func TestRender(t *testing.T) {
	cases := []struct {
		name  string
		spans []tracetree.Span
		want  string
	}{
		{
			name:  "single span",
			spans: []tracetree.Span{named("r", "", "api", "GET /healthz", 0, 2)},
			want: `trace 4bf92f3577b34da6a3ce929d0e0e4736
* api: GET /healthz start=0s dur=2ms self=2ms
`,
		},
		{
			name: "README example",
			spans: []tracetree.Span{
				named("checkout", "", "frontend", "POST /checkout", 0, 500),
				named("auth", "checkout", "auth", "Authorize", 5, 25),
				named("cart", "checkout", "cart", "GetCart", 30, 80),
				named("inventory", "checkout", "inventory", "Reserve", 85, 200),
				named("db", "inventory", "postgres", "UPDATE stock", 90, 190),
				named("pricing", "checkout", "pricing", "Quote", 85, 150),
				named("payment", "checkout", "payment", "Charge", 210, 480),
				named("fraud", "payment", "fraud", "Score", 215, 300),
				named("psp", "payment", "payment", "POST /v1/capture", 300, 470),
			},
			want: `trace 4bf92f3577b34da6a3ce929d0e0e4736
* frontend: POST /checkout start=0s dur=500ms self=45ms
*   auth: Authorize start=5ms dur=20ms self=20ms
*   cart: GetCart start=30ms dur=50ms self=50ms
*   inventory: Reserve start=85ms dur=115ms self=15ms
*     postgres: UPDATE stock start=90ms dur=100ms self=100ms
    pricing: Quote start=85ms dur=65ms self=65ms
*   payment: Charge start=210ms dur=270ms self=15ms
*     fraud: Score start=215ms dur=85ms self=85ms
*     payment: POST /v1/capture start=300ms dur=170ms self=170ms
`,
		},
		{
			// checkoutSpans() in helpers_test.go, with an orphan tree.
			name:  "checkout with an orphan, durations over 1s",
			spans: checkoutSpans(),
			want: `trace 4bf92f3577b34da6a3ce929d0e0e4736
* frontend: POST /checkout start=0s dur=1.25s self=75ms
*   auth: Authorize start=10ms dur=30ms self=30ms
*   cart: GetCart start=45ms dur=75ms self=65ms
*     redis: GET start=50ms dur=10ms self=10ms
*   inventory: Reserve start=125ms dur=275ms self=15ms
*     postgres: UPDATE stock start=130ms dur=260ms self=260ms
    pricing: Quote start=125ms dur=175ms self=175ms
*   payment: Charge start=410ms dur=790ms self=15ms
*     fraud: Score start=415ms dur=85ms self=85ms
*     payment: POST /v1/capture start=500ms dur=690ms self=690ms
*   frontend: publish receipt start=1.21s dur=5ms self=5ms
orphan: parent notify1 not in trace
  mailer: SendReceipt start=1.3s dur=150ms self=20ms
    mailer: smtp DATA start=1.31s dur=130ms self=130ms
`,
		},
		{
			// The cache branch is not on the critical path, so neither is its
			// child. Orphan cleanup started before the root: negative start.
			name: "branch off the critical path, two orphans",
			spans: []tracetree.Span{
				named("r", "", "api", "GET /orders", 0, 100),
				named("db", "r", "postgres", "SELECT orders", 10, 90),
				named("cache", "r", "redis", "GET orders", 10, 40),
				named("conn", "cache", "redis", "dial", 12, 20),
				named("o2", "gone2", "worker", "process", 50, 80),
				named("o1", "gone1", "cron", "cleanup", -20, 30),
				named("o1a", "o1", "cron", "DELETE expired", -10, 20),
			},
			want: `trace 4bf92f3577b34da6a3ce929d0e0e4736
* api: GET /orders start=0s dur=100ms self=20ms
    redis: GET orders start=10ms dur=30ms self=22ms
      redis: dial start=12ms dur=8ms self=8ms
*   postgres: SELECT orders start=10ms dur=80ms self=80ms
orphan: parent gone1 not in trace
  cron: cleanup start=-20ms dur=50ms self=20ms
    cron: DELETE expired start=-10ms dur=30ms self=30ms
orphan: parent gone2 not in trace
  worker: process start=50ms dur=30ms self=30ms
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := mustBuild(t, tc.spans)
			checkGolden(t, tracetree.Render(tr), tc.want)
		})
	}
}

// checkGolden compares Render output with the expected text and reports the
// first line that differs.
func checkGolden(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("Render() output does not end with \"\\n\"; every line, also the last one, ends with a newline")
	}
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(want, "\n")
	line := func(lines []string, i int) string {
		if i < len(lines) {
			return fmt.Sprintf("%q", lines[i])
		}
		return "(no such line)"
	}
	for i := range max(len(gotLines), len(wantLines)) {
		if g, w := line(gotLines, i), line(wantLines, i); g != w {
			t.Errorf("Render() differs at line %d:\n got  %s\n want %s\n\nfull output:\n%s\nwant:\n%s",
				i+1, g, w, got, want)
			return
		}
	}
}
