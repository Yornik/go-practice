package tableprint_test

import (
	"fmt"
	"os"
	"time"

	tableprint "github.com/yornik/go-practice/exercises/18-tableprint"
)

func ExampleRender() {
	type feeder struct {
		Name     string
		Firmware string        `table:"FW"`
		Battery  *int          `table:"BATTERY%"`
		LastSeen time.Duration `table:"LAST SEEN"`
		apiToken string
	}
	battery := 87
	rows := []feeder{
		{"hq-window-01", "2.10.0", &battery, 12 * time.Second, "tok-8f3a91"},
		{"feeder-0412", "2.9.3", nil, 41 * time.Minute, "tok-c27d40"},
	}
	if err := tableprint.Render(os.Stdout, rows); err != nil {
		fmt.Println(err)
	}
	// Output:
	// NAME          FW      BATTERY%  LAST SEEN
	// hq-window-01  2.10.0  87        12s
	// feeder-0412   2.9.3   <none>    41m0s
}
