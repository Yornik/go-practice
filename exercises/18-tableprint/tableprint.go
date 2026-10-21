// Package tableprint prints a slice of structs as an aligned text table, the
// way kubectl get and docker ps print their results:
//
//	NAME          DRV     GPU UTIL  LAST SEEN
//	canary-h3-01  2.10.0  87        12s
//	vl-gpu-0412   2.9.3   <none>    41m0s
//
// The columns come from the struct type and its `table` tags. The README
// lists the exact rules.
package tableprint

import (
	"errors"
	"io"
)

// ErrNotStructs is wrapped by the error that Headers and Render return when
// rows is not a slice of structs or a slice of pointers to structs. Check for
// it with errors.Is.
var ErrNotStructs = errors.New("tableprint: rows must be a slice of structs")

// ErrNilRow is wrapped by the error that Render returns when an element of a
// slice of pointers is nil. Check for it with errors.Is.
var ErrNilRow = errors.New("tableprint: nil row")

// Headers returns the column headers for rows, 1 per column, in the order of
// the struct fields.
//
// rows must be a slice whose element type is a struct or a pointer to a
// struct. The columns are the exported fields of that struct. The tag
// `table:"NAME"` sets the header, `table:"-"` hides the field, and a field
// without a tag gets its name in upper case. Headers looks only at the type
// of rows, never at the elements: an empty slice and a nil row are fine.
//
// For any other rows it returns nil and an error that wraps ErrNotStructs and
// contains the type of rows as %T prints it, for example []int.
func Headers(rows any) ([]string, error) {
	panic("TODO: implement")
}

// Render writes rows to w as a table: a header row (the result of Headers),
// then 1 row per element of rows, in order.
//
// Every column is as wide as its widest cell or header, counted in runes, and
// 2 spaces separate the columns. The last column is not padded, and every row
// ends with "\n". An empty slice gives only the header row. The README lists
// how each field value becomes a cell.
//
// Render returns an error that wraps ErrNotStructs for rows that Headers
// rejects, and an error that wraps ErrNilRow and contains "rows[i]" when the
// element at index i is a nil pointer. In both cases it does not call
// w.Write. An error from w comes back so that errors.Is finds it.
func Render(w io.Writer, rows any) error {
	panic("TODO: implement")
}
