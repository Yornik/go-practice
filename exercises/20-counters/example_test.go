package counters_test

import (
	"fmt"
	"os"

	counters "github.com/yornik/go-practice/exercises/20-counters"
)

func ExampleRegistry_Write() {
	r := counters.NewRegistry()
	r.Counter("requests_lightrail").Add(3)
	r.Counter("requests_spire").Add(2)
	r.Counter("requests_lightrail").Add(1)
	r.Counter("rejected") // created with the value 0

	if err := r.Write(os.Stdout); err != nil {
		fmt.Println("error:", err)
	}
	// Output:
	// rejected 0
	// requests_lightrail 4
	// requests_spire 2
}

func ExampleRegistry_Drain() {
	r := counters.NewRegistry()
	lightrail := r.Counter("requests_lightrail")
	lightrail.Add(4)
	r.Counter("requests_spire").Add(2)

	fmt.Println(r.Drain())
	fmt.Println(r.Snapshot())
	lightrail.Add(5)
	fmt.Println(r.Snapshot())
	// Output:
	// [{requests_lightrail 4} {requests_spire 2}]
	// [{requests_lightrail 0} {requests_spire 0}]
	// [{requests_lightrail 5} {requests_spire 0}]
}
