package grep_test

import (
	"fmt"
	"os"
	"strings"

	grep "github.com/yornik/go-practice/exercises/04-grep"
)

// Like grep -C 1 ERROR app.log.
func ExampleFilter() {
	log := `level=info msg="GET /api/orders" status=200
level=info msg="GET /api/orders" status=200
level=error msg="db timeout" status=500
level=info msg="retrying"
level=info msg="GET /api/users" status=200
level=info msg="GET /api/users" status=200
level=error msg="db timeout" status=500
level=info msg="giving up"`

	st, err := grep.Filter(strings.NewReader(log), os.Stdout, grep.Options{
		Match:  func(line string) bool { return strings.Contains(line, "level=error") },
		Before: 1,
		After:  1,
	})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%d lines, %d matches, %d bytes read\n", st.Lines, st.Matches, st.BytesRead)
	// Output:
	// level=info msg="GET /api/orders" status=200
	// level=error msg="db timeout" status=500
	// level=info msg="retrying"
	// --
	// level=info msg="GET /api/users" status=200
	// level=error msg="db timeout" status=500
	// level=info msg="giving up"
	// 8 lines, 2 matches, 306 bytes read
}
