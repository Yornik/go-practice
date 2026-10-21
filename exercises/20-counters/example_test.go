package counters_test

import (
	"fmt"
	"os"

	counters "github.com/yornik/go-practice/exercises/20-counters"
)

func ExampleRegistry_Write() {
	r := counters.NewRegistry()
	r.Counter("uploads_eu").Add(3)
	r.Counter("uploads_us").Add(2)
	r.Counter("uploads_eu").Add(1)
	r.Counter("rejected") // created with the value 0

	if err := r.Write(os.Stdout); err != nil {
		fmt.Println("error:", err)
	}
	// Output:
	// rejected 0
	// uploads_eu 4
	// uploads_us 2
}

func ExampleRegistry_Drain() {
	r := counters.NewRegistry()
	eu := r.Counter("uploads_eu")
	eu.Add(4)
	r.Counter("uploads_us").Add(2)

	fmt.Println(r.Drain())
	fmt.Println(r.Snapshot())
	eu.Add(5)
	fmt.Println(r.Snapshot())
	// Output:
	// [{uploads_eu 4} {uploads_us 2}]
	// [{uploads_eu 0} {uploads_us 0}]
	// [{uploads_eu 5} {uploads_us 0}]
}
