// Package statuspage models a public status page: the status of each
// component, the overall status of the page, and the banner that customers
// read at the top of it.
package statuspage

import "errors"

// Status is the state of 1 component. String uses the component statuses of
// the Statuspage API, plus "unknown", which that API does not have.
type Status int

// The 6 statuses. The zero value is Unknown: nobody has reported yet.
//
// The numeric order is not the order of severity. From least to most severe
// the order is Operational, Maintenance, Unknown, Degraded, PartialOutage,
// MajorOutage.
const (
	Unknown Status = iota // the zero value: nobody has reported
	Operational
	Maintenance
	Degraded
	PartialOutage
	MajorOutage
)

// ErrUnknownStatus is wrapped by the error ParseStatus returns for a word it
// does not know, and by the error Set returns for a Status outside the 6
// constants. Check for it with errors.Is.
var ErrUnknownStatus = errors.New("statuspage: unknown status")

// ErrUnknownComponent is wrapped by the error Set returns for a component
// that is not on the page. Check for it with errors.Is.
var ErrUnknownComponent = errors.New("statuspage: unknown component")

// String returns "unknown", "operational", "under_maintenance",
// "degraded_performance", "partial_outage" or "major_outage". For any other
// value it returns "Status(N)", for example "Status(42)".
func (s Status) String() string {
	panic("TODO: implement")
}

// ParseStatus converts a word to a Status. It accepts the 6 words that String
// returns and the aliases "maintenance" and "degraded", in any letter case,
// and nothing else.
//
// For any other input it returns Unknown and an error that wraps
// ErrUnknownStatus and contains the input quoted with %q, for example:
// statuspage: unknown status "ok".
func ParseStatus(s string) (Status, error) {
	panic("TODO: implement")
}

// Page is a status page: a fixed set of components, each with a Status.
//
// The zero value is a page without components. A Page is not safe for
// concurrent use.
type Page struct {
	// You design the fields.
}

// NewPage returns a page with the given components, each with the status
// Unknown. A name that appears more than once is 1 component. Names are
// compared exactly, letter case included.
func NewPage(components ...string) *Page {
	panic("TODO: implement")
}

// Set changes the status of 1 component and returns nil.
//
// It returns an error and changes nothing if the component is not on the
// page (the error wraps ErrUnknownComponent and contains the name quoted
// with %q) or if s is not 1 of the 6 constants (the error wraps
// ErrUnknownStatus). The component is checked first.
func (p *Page) Set(component string, s Status) error {
	panic("TODO: implement")
}

// Get returns the status of a component and true, or Unknown and false if
// the component is not on the page.
func (p *Page) Get(component string) (Status, bool) {
	panic("TODO: implement")
}

// Overall returns the most severe status of all components, and Unknown for
// a page without components.
func (p *Page) Overall() Status {
	panic("TODO: implement")
}

// Banner returns the text at the top of the page:
//
//   - "All systems operational" when every component is Operational;
//   - "Status unknown" for a page without components;
//   - otherwise 1 group per status that a component has, except Operational,
//     most severe first, joined with "; ". A group is the label of the
//     status, ": " and its components from A to Z joined with ", ", for
//     example "Major outage: Photo uploads; Status unknown: API, Checkout".
func (p *Page) Banner() string {
	panic("TODO: implement")
}
