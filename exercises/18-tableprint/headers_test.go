package tableprint_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tableprint "github.com/yornik/go-practice/exercises/18-tableprint"
)

func TestHeaders(t *testing.T) {
	type node struct {
		Name     string
		Driver   string `table:"DRV"`
		GPUUtil  int    `table:"GPU UTIL"`
		LastSeen string `table:"LAST SEEN"`
	}
	type withHidden struct {
		Name   string
		Token  string `table:"-"`
		secret string
		owner  string `table:"OWNER"`
		Zone   string
	}
	type tagSpelling struct {
		Restarts int    `table:"Restarts"`
		Zone     string `json:"zone"`
		Region   string `json:"region" table:"AZ"`
		Node     string `table:""`
	}
	type withEmbedded struct {
		Meta
		Phase string
	}
	type withEmbeddedPointer struct {
		*Meta
		Phase string
	}
	type withUnexportedEmbedded struct {
		meta
		Phase string
	}

	tests := []struct {
		name string
		rows any
		want []string
	}{
		{"no tag gives the field name in upper case", []pod{{"checkout-7f9c4", "Running", 0}}, []string{"NAME", "STATUS", "RESTARTS"}},
		{"a tag sets the header", []node{{}}, []string{"NAME", "DRV", "GPU UTIL", "LAST SEEN"}},
		{"table:\"-\" and unexported fields are not columns", []withHidden{{}}, []string{"NAME", "ZONE"}},
		{"a tag is used as written, other tag keys are ignored, an empty tag is no tag", []tagSpelling{{}}, []string{"Restarts", "ZONE", "AZ", "NODE"}},
		{"an embedded struct is 1 column named after its type", []withEmbedded{{}}, []string{"META", "PHASE"}},
		{"an embedded pointer is 1 column named after its type", []withEmbeddedPointer{{}}, []string{"META", "PHASE"}},
		{"an embedded unexported type is not a column", []withUnexportedEmbedded{{}}, []string{"PHASE"}},
		{"empty slice", []node{}, []string{"NAME", "DRV", "GPU UTIL", "LAST SEEN"}},
		{"nil slice", []node(nil), []string{"NAME", "DRV", "GPU UTIL", "LAST SEEN"}},
		{"slice of pointers", []*pod{{"checkout-7f9c4", "Running", 0}}, []string{"NAME", "STATUS", "RESTARTS"}},
		{"slice of pointers with a nil row", []*pod{nil}, []string{"NAME", "STATUS", "RESTARTS"}},
		{"empty slice of pointers", []*node{}, []string{"NAME", "DRV", "GPU UTIL", "LAST SEEN"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			got, err := tableprint.Headers(tt.rows)
			if err != nil {
				t.Fatalf("Headers(%#v) error = %v, want nil", tt.rows, err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Headers(%#v)\n got  %q\n want %q", tt.rows, got, tt.want)
			}
		})
	}
}

// notStructs are inputs that are not a slice of structs or of pointers to
// structs. TestRenderErrorsNotStructs uses them too.
func notStructs() []struct {
	name string
	rows any
} {
	type node struct{ Name string }
	rows := []node{{"canary-h3-01"}}
	return []struct {
		name string
		rows any
	}{
		{"nil", nil},
		{"a struct", node{"canary-h3-01"}},
		{"a pointer to a struct", &node{"canary-h3-01"}},
		{"an array of structs", [2]node{}},
		{"a map of structs", map[string]node{"a": {"canary-h3-01"}}},
		{"a slice of ints", []int{1, 2}},
		{"a slice of pointers to ints", []*int{nil}},
		{"a slice of strings", []string(nil)},
		{"a slice of any that holds structs", []any{node{"canary-h3-01"}}},
		{"a pointer to a slice of structs", &rows},
	}
}

func TestHeadersErrors(t *testing.T) {
	for _, tt := range notStructs() {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			got, err := tableprint.Headers(tt.rows)
			if !errors.Is(err, tableprint.ErrNotStructs) {
				t.Fatalf("Headers(%#v) error = %v, want an error that wraps ErrNotStructs", tt.rows, err)
			}
			if typ := fmt.Sprintf("%T", tt.rows); !strings.Contains(err.Error(), typ) {
				t.Errorf("Headers(%#v) error = %q, want it to contain the type %q", tt.rows, err, typ)
			}
			if got != nil {
				t.Errorf("Headers(%#v) = %q, want nil with the error", tt.rows, got)
			}
		})
	}
}
