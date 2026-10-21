package nightlog_test

import (
	"fmt"
	"os"
	"strings"
	"time"

	nightlog "github.com/yornik/go-practice/exercises/11-nightlog"
)

func ExampleWrite() {
	log := `2026-10-16T03:10:01Z INFO heartbeat gpu_temp_c=41
2026-10-16T03:11:01Z INFO heartbeat gpu_temp_c=41
2026-10-16T03:11:58Z info batch queue=inference
2026-10-16T03:12:09Z WARN upload slow ms=2140
2026-10-16T03:12:

U-Boot 2024.01 (Sep 30 2026 - 11:20:07 +0000)
2026-10-16T03:59:41Z INFO boot drv=2.10.0 reason=power-on
2026-10-16T04:00:02Z ERROR gpud: fan controller failed
`
	s, err := nightlog.Summarize(strings.NewReader(log), 10*time.Minute)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	if err := nightlog.Write(os.Stdout, s); err != nil {
		fmt.Println("error:", err)
	}
	// Output:
	// lines:     9
	// malformed: 2
	// first:     2026-10-16T03:10:01Z
	// last:      2026-10-16T04:00:02Z
	// levels:    ERROR=1 INFO=4 WARN=1
	// gap:       2026-10-16T03:12:09Z to 2026-10-16T03:59:41Z (47m32s)
}
