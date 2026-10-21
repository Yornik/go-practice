package fanout_test

import (
	"errors"
	"fmt"

	fanout "github.com/yornik/go-practice/exercises/19-fanout"
)

func ExampleRunAll() {
	checks := []fanout.Check{
		{Name: "checkout", Run: func() error { return nil }},
		{Name: "orders", Run: func() error { return errors.New("GET /healthz: 503 Service Unavailable") }},
		{Name: "hush-api", Run: func() error { panic("empty body") }},
	}
	for _, r := range fanout.RunAll(checks) {
		if r.Err != nil {
			fmt.Printf("%-12s FAIL %v\n", r.Name, r.Err)
			continue
		}
		fmt.Printf("%-12s ok\n", r.Name)
	}
	// Output:
	// checkout     ok
	// orders       FAIL GET /healthz: 503 Service Unavailable
	// hush-api     FAIL fanout: check panicked: empty body
}
