package checkdisk_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	checkdisk "github.com/yornik/go-practice/exercises/15-checkdisk"
)

// timeout bounds every call to Run, so a loop that never ends fails the
// test with a message instead of hanging the test binary.
const timeout = 5 * time.Second

// header is the first line that `df -P` prints.
const header = "Filesystem     1024-blocks      Used Available Capacity Mounted on"

// df returns header and lines as `df -P` output: 1 line each, every line
// ending with "\n".
func df(lines ...string) string {
	return header + "\n" + strings.Join(append(lines, ""), "\n")
}

// row returns 1 line of `df -P` output for a file system of 1000 blocks
// that is pct percent full: row("/var", 91) is
// "/dev/sda1 1000 910 90 91% /var".
func row(mount string, pct int) string {
	return fmt.Sprintf("/dev/sda1 1000 %d %d %d%% %s", pct*10, 1000-pct*10, pct, mount)
}

// readFile returns the content of a file in testdata/.
func readFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// result is what 1 call to Run produced.
type result struct {
	code   int
	stdout string
	stderr string
}

// run calls checkdisk.Run with args and stdin, and with in-memory stdout and
// stderr. It runs Run in a goroutine and fails the test if Run panics (with
// the stack, so you can find the line) or does not return within the
// timeout.
func run(t *testing.T, stdin io.Reader, args ...string) result {
	t.Helper()
	type outcome struct {
		res   result
		panic string
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- outcome{panic: fmt.Sprintf("%v\n%s", v, debug.Stack())}
			}
		}()
		var stdout, stderr bytes.Buffer
		code := checkdisk.Run(args, stdin, &stdout, &stderr)
		done <- outcome{res: result{code: code, stdout: stdout.String(), stderr: stderr.String()}}
	}()
	select {
	case o := <-done:
		if o.panic != "" {
			t.Fatalf("Run(%s) panicked: %s", showArgs(args), o.panic)
		}
		return o.res
	case <-time.After(timeout):
		t.Fatalf("Run(%s) did not return within %v (does the loop stop at the end of stdin?)", showArgs(args), timeout)
		return result{}
	}
}

// runText is run with stdin read from the string input.
func runText(t *testing.T, input string, args ...string) result {
	t.Helper()
	return run(t, strings.NewReader(input), args...)
}

// showArgs formats args for failure messages: ["-w" "50" "-x" "/boot"].
func showArgs(args []string) string {
	if len(args) == 0 {
		return "[]"
	}
	return fmt.Sprintf("%q", args)
}

// call describes a Run call with its input for failure messages.
func call(args []string, input string) string {
	return fmt.Sprintf("Run(%s) with stdin\n%s", showArgs(args), indent(input))
}

// indent prefixes every line of s with 4 spaces and marks the end of s, so
// blank lines and a missing last "\n" are visible.
func indent(s string) string {
	if s == "" {
		return "    (empty)"
	}
	return "    " + strings.ReplaceAll(s, "\n", "\n    ") + "(end of input)"
}

// stateNames are the words that the status line uses for each exit code.
var stateNames = map[int]string{
	checkdisk.OK:       "OK",
	checkdisk.Warning:  "WARNING",
	checkdisk.Critical: "CRITICAL",
	checkdisk.Unknown:  "UNKNOWN",
}

// checkUnknown fails the test unless res is an UNKNOWN result: exit code 3
// and exactly 1 line "DISK UNKNOWN - <reason>" on stdout, with a reason that
// is not empty. It returns the reason.
func checkUnknown(t *testing.T, what string, res result) string {
	t.Helper()
	if res.code != checkdisk.Unknown {
		t.Errorf("%s\nreturned exit code %d, want %d (Unknown)", what, res.code, checkdisk.Unknown)
	}
	const prefix = "DISK UNKNOWN - "
	reason, ok := strings.CutPrefix(res.stdout, prefix)
	switch {
	case !ok:
		t.Errorf("%s\nwrote to stdout %q, want a line that starts with %q", what, res.stdout, prefix)
	case !strings.HasSuffix(reason, "\n") || strings.Count(reason, "\n") != 1:
		t.Errorf("%s\nwrote to stdout %q, want exactly 1 line that ends with \"\\n\"", what, res.stdout)
	case strings.TrimSpace(reason) == "":
		t.Errorf("%s\nwrote to stdout %q, want a reason after %q", what, res.stdout, prefix)
	}
	return reason
}

// checkLineNumber fails the test unless reason names line n as "line <n>".
func checkLineNumber(t *testing.T, what string, reason string, n int) {
	t.Helper()
	re := regexp.MustCompile(fmt.Sprintf(`\bline %d\b`, n))
	if !re.MatchString(reason) {
		t.Errorf("%s\nwrote the reason %q, want it to contain \"line %d\"", what, strings.TrimSpace(reason), n)
	}
}

// checkLine fails the test unless Run returned code, wrote exactly want and
// "\n" to stdout, and wrote nothing to stderr.
func checkLine(t *testing.T, what string, res result, code int, want string) {
	t.Helper()
	if res.code != code {
		t.Errorf("%s\nreturned exit code %d, want %d (%s)", what, res.code, code, stateNames[code])
	}
	if res.stdout != want+"\n" {
		t.Errorf("%s\nwrote to stdout\n    %q\nwant\n    %q", what, res.stdout, want+"\n")
	}
	if res.stderr != "" {
		t.Errorf("%s\nwrote to stderr %q, want nothing", what, res.stderr)
	}
}

// errReader returns data and then fails every Read with err.
func errReader(data string, err error) io.Reader {
	return io.MultiReader(strings.NewReader(data), iotest.ErrReader(err))
}
