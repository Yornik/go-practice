// Package waves plans a rollout in waves. Order sorts host names the way
// people read them (node-9 before node-10), Without drops the hosts to
// skip, and Waves cuts a list of hosts into a canary wave and waves of a
// fixed size:
//
//	plan := Order(Without(hall2, skip))
//	w, err := Waves(plan, 1, 50)
//
// Every function returns slices that the caller owns. Changing or appending
// to a result never changes the input, and changing the input later never
// changes a result.
package waves

// Order returns the names in natural order, in a new slice.
//
// A name is a prefix followed by a number: the run of ASCII digits at the
// end of the name, as long as possible. The run may be empty. 2 names
// compare by their prefixes in plain string order first. For equal
// prefixes a name without a number comes first, then the smaller number
// value, then plain string order:
//
//	checkout, node-1, node-007, node-7, node-9, node-10, web, web1
//
// Order does not modify names. Changing an element of the result, or
// appending to it, never changes names. For empty input it returns nil or
// an empty slice.
func Order(names []string) []string {
	panic("TODO: implement")
}

// Without returns the names that are not in skip, in the order of names,
// in a new slice. Names compare byte by byte. Every copy of a name that is
// in skip is dropped; every copy of any other name is kept.
//
// Without never writes into the array behind names, not even into its
// spare capacity beyond len(names). Changing an element of the result, or
// appending to it, never changes names. For an empty result it returns nil
// or an empty slice.
func Without(names, skip []string) []string {
	panic("TODO: implement")
}

// Waves splits names into a canary wave of the first canary names (none
// when canary is 0), then waves of size names, in the order of names. The
// last wave may be shorter. No wave is empty. When canary is at least
// len(names), there is 1 wave with every name.
//
// It returns nil and an error that contains the bad value if canary < 0 or
// size < 1, also when names is empty. Empty names with valid arguments give
// no waves and a nil error.
//
// Every wave is independent: appending to a wave or changing its elements
// never changes another wave or names (not even the spare capacity of
// names), and changing names never changes a wave.
func Waves(names []string, canary, size int) ([][]string, error) {
	panic("TODO: implement")
}
