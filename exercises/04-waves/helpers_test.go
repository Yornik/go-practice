package waves_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	waves "github.com/yornik/go-practice/exercises/04-waves"
)

// timeout bounds a call to Waves in TestWavesErrors. A correct Waves needs
// microseconds.
const timeout = 5 * time.Second

// callWaves calls Waves in a goroutine and waits at most timeout. A loop
// that never ends, such as one that steps through names by a size of 0,
// would otherwise run until the machine runs out of memory. After timeout,
// callWaves names the call and stops the test binary.
//
// A panic in Waves, such as the "TODO: implement" of the stub, fails the
// test with the panic value.
func callWaves(t *testing.T, names []string, canary, size int) ([][]string, error) {
	t.Helper()
	type result struct {
		waves    [][]string
		err      error
		panicked any
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if v := recover(); v != nil {
				done <- result{panicked: v}
			}
		}()
		w, err := waves.Waves(names, canary, size)
		done <- result{waves: w, err: err}
	}()

	select {
	case r := <-done:
		if r.panicked != nil {
			t.Fatalf("Waves(%q, %d, %d) panicked: %v", names, canary, size, r.panicked)
		}
		return r.waves, r.err
	case <-time.After(timeout):
		fmt.Fprintf(os.Stderr, "\n%s: Waves(%q, %d, %d) did not return within %v. "+
			"Does a loop never end for this size?\n", t.Name(), names, canary, size, timeout)
		os.Exit(2)
		return nil, nil
	}
}
