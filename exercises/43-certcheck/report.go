package certcheck

import "io"

// WriteReport writes rs to w as a text table with aligned columns, the most
// urgent rows first: STATUS, NAME, EXPIRES, DAYS and DETAIL. The exact format
// and order are described in the README. WriteReport does not reorder rs. It
// returns the first error from w.
func WriteReport(w io.Writer, rs []Result) error {
	panic("TODO: implement")
}
