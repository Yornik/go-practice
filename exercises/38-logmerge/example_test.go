package logmerge_test

import (
	"fmt"
	"iter"
	"strings"

	logmerge "github.com/yornik/go-practice/exercises/38-logmerge"
)

// Example_pipeline puts ParseKubectl and Merge together: the saved output of
// kubectl logs --timestamps for two pods, merged into one timeline. kubectl
// output is already in order per pod, so it needs no Reorder.
func Example_pipeline() {
	api := strings.NewReader(`2026-10-16T12:00:00.100Z GET /orders/17 200
2026-10-16T12:00:01.900Z GET /orders/18 500
2026-10-16T12:00:02.000Z GET /orders/19 500
`)
	db := strings.NewReader(`2026-10-16T12:00:01.200Z connection pool exhausted
--- the log was rotated here ---
2026-10-16T12:00:01.950Z primary unreachable, failing over
`)
	bad := 0
	// entries adapts a ParseKubectl sequence to Merge: it counts and drops
	// the parse errors.
	entries := func(seq iter.Seq2[logmerge.Entry, error]) iter.Seq[logmerge.Entry] {
		return func(yield func(logmerge.Entry) bool) {
			for e, err := range seq {
				if err != nil {
					bad++
					continue
				}
				if !yield(e) {
					return
				}
			}
		}
	}

	merged := logmerge.Merge(
		entries(logmerge.ParseKubectl("api", api)),
		entries(logmerge.ParseKubectl("db", db)),
	)
	for e := range merged {
		fmt.Printf("%s %-3s %s\n", e.Time.Format("15:04:05.000"), e.Source, e.Line)
	}
	fmt.Println("lines skipped:", bad)
	// Output:
	// 12:00:00.100 api GET /orders/17 200
	// 12:00:01.200 db  connection pool exhausted
	// 12:00:01.900 api GET /orders/18 500
	// 12:00:01.950 db  primary unreachable, failing over
	// 12:00:02.000 api GET /orders/19 500
	// lines skipped: 1
}
