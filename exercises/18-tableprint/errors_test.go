package tableprint_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tableprint "github.com/yornik/go-practice/exercises/18-tableprint"
)

func TestRenderErrorsNotStructs(t *testing.T) {
	for _, tt := range notStructs() {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			var w recordingWriter
			err := tableprint.Render(&w, tt.rows)
			if !errors.Is(err, tableprint.ErrNotStructs) {
				t.Fatalf("Render(%#v) error = %v, want an error that wraps ErrNotStructs", tt.rows, err)
			}
			if typ := fmt.Sprintf("%T", tt.rows); !strings.Contains(err.Error(), typ) {
				t.Errorf("Render(%#v) error = %q, want it to contain the type %q", tt.rows, err, typ)
			}
			if w.calls != 0 || w.Len() != 0 {
				t.Errorf("Render(%#v) made %d Write calls and wrote %q, want no writes", tt.rows, w.calls, w.String())
			}
		})
	}
}

func TestRenderErrorsNilRow(t *testing.T) {
	tests := []struct {
		name  string
		rows  []*pod
		index string
	}{
		{"the only row is nil", []*pod{nil}, "rows[0]"},
		{"the last row is nil", []*pod{{"checkout-7f9c4", "Running", 0}, {"orders-5d8b2", "Pending", 1}, nil}, "rows[2]"},
		{"a row in the middle is nil", []*pod{{"checkout-7f9c4", "Running", 0}, nil, {"orders-5d8b2", "Pending", 1}}, "rows[1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			var w recordingWriter
			err := tableprint.Render(&w, tt.rows)
			if !errors.Is(err, tableprint.ErrNilRow) {
				t.Fatalf("Render(%#v) error = %v, want an error that wraps ErrNilRow", tt.rows, err)
			}
			if !strings.Contains(err.Error(), tt.index) {
				t.Errorf("Render(%#v) error = %q, want it to contain %q", tt.rows, err, tt.index)
			}
			if w.calls != 0 || w.Len() != 0 {
				t.Errorf("Render(%#v) made %d Write calls and wrote %q, want no writes", tt.rows, w.calls, w.String())
			}
		})
	}
}

func TestRenderErrorsWriter(t *testing.T) {
	errDiskFull := errors.New("write /var/report.txt: no space left on device")
	rows := []pod{
		{"checkout-7f9c4", "Running", 0},
		{"orders-5d8b2", "CrashLoopBackOff", 12},
	}
	tests := []struct {
		name  string
		limit int
	}{
		{"the first write fails", 0},
		{"a write fails after 10 bytes", 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			w := &failingWriter{limit: tt.limit, err: errDiskFull}
			err := tableprint.Render(w, rows)
			if !errors.Is(err, errDiskFull) {
				t.Errorf("Render to a writer that fails after %d bytes: error = %v, want an error that wraps %q",
					tt.limit, err, errDiskFull)
			}
		})
	}
}
