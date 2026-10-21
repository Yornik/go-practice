package promtext

import "io"

// SeriesKey returns the canonical text of a series: the name, then the labels
// sorted by name in braces, as in
//
//	http_requests_total{code="200",method="GET"}
//
// Label values are escaped as in the text format (\\, \" and \n). Without
// labels the key is just the name. SeriesKey does not modify labels.
func SeriesKey(name string, labels []Label) string {
	panic("TODO: implement")
}

// Write writes the families in the canonical text format: for each family an
// optional HELP line, a TYPE line, then one line per sample. The README
// lists the exact format. Parse(Write(fams)) gives fams back for any fams
// that Parse returned.
//
// Write returns the first error from w. It does not modify fams.
func Write(w io.Writer, fams []Family) error {
	panic("TODO: implement")
}
