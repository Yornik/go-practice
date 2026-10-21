package tracetree

// Render returns the trace as indented text: a header line, one line per
// node of the root tree, then each orphan tree under its own header line.
// Nodes on the critical path are marked with "*". The README defines the
// exact format. Like the functions in analyze.go, Render takes a valid tree
// and does not modify it.
func Render(t *Trace) string {
	panic("TODO: implement")
}
