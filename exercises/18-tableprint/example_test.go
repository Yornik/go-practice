package tableprint_test

import (
	"fmt"
	"os"
	"time"

	tableprint "github.com/yornik/go-practice/exercises/18-tableprint"
)

func ExampleRender() {
	type node struct {
		Name     string
		Driver   string        `table:"DRV"`
		GPUUtil  *int          `table:"GPU UTIL"`
		LastSeen time.Duration `table:"LAST SEEN"`
		apiToken string
	}
	gpuUtil := 87
	rows := []node{
		{"canary-h3-01", "2.10.0", &gpuUtil, 12 * time.Second, "tok-8f3a91"},
		{"vl-gpu-0412", "2.9.3", nil, 41 * time.Minute, "tok-c27d40"},
	}
	if err := tableprint.Render(os.Stdout, rows); err != nil {
		fmt.Println(err)
	}
	// Output:
	// NAME          DRV     GPU UTIL  LAST SEEN
	// canary-h3-01  2.10.0  87        12s
	// vl-gpu-0412   2.9.3   <none>    41m0s
}
