// Package deployorder plans deployments from the dependencies that services
// declare: it groups services into waves that can be deployed in parallel,
// reports dependency cycles, and answers "what breaks if this service goes
// down?" and "why does this service need that one?".
package deployorder

import "errors"

// Graph holds a set of services and, for each one, the services it depends
// on. "a depends on b" means b must be running before a starts.
//
// A Graph does not change after New. All its methods are safe to call from
// several goroutines at once.
type Graph struct {
	// TODO: add fields.
}

// New builds a Graph from deps, which maps every service to the services it
// depends on. A nil or empty slice means no dependencies; a nil or empty map
// is a valid, empty graph.
//
// Every dependency must also be a key of deps. If one is not, New returns a
// nil Graph and an *UnknownDependencyError for the first unknown dependency:
// services are checked in sorted order, and the dependencies of each service
// in sorted order.
//
// Duplicate dependencies count once. New accepts cycles, including a service
// that depends on itself: Waves and Order report them.
//
// New does not modify deps, and the Graph shares no memory with it: changing
// deps after New does not change the Graph.
func New(deps map[string][]string) (*Graph, error) {
	panic("TODO: implement")
}

// UnknownDependencyError reports a dependency that is not a service of the
// graph, usually a typo or a service that was removed.
type UnknownDependencyError struct {
	Service    string // the service that declares the dependency
	Dependency string // the dependency that is not a key of the map
}

// Error returns, for Service "api" and Dependency "postgress":
//
//	service "api" depends on unknown service "postgress"
func (e *UnknownDependencyError) Error() string {
	panic("TODO: implement")
}

// CycleError reports a dependency cycle: services that can never be deployed,
// because each one waits for the next.
type CycleError struct {
	// Path lists the services of the cycle, each one depending on the next.
	// It starts and ends with the same service: [a b c a] means a depends on
	// b, b depends on c and c depends on a. A service that depends on itself
	// gives [a a]. The README states which cycle is reported when there are
	// several.
	Path []string
}

// Error returns "dependency cycle: " followed by Path joined with " -> ", for
// example:
//
//	dependency cycle: a -> b -> c -> a
func (e *CycleError) Error() string {
	panic("TODO: implement")
}

// ErrUnknownService is wrapped by the error that Dependents returns for a
// service that is not in the graph. Check for it with errors.Is.
var ErrUnknownService = errors.New("unknown service")

// Waves groups the services into deployment waves. Wave 0 holds the services
// without dependencies. Every other service is in the earliest wave that comes
// after the waves of all its dependencies. Each wave is sorted.
//
// An empty graph has no waves. If the graph has a cycle, Waves returns nil and
// a *CycleError.
func (g *Graph) Waves() ([][]string, error) {
	panic("TODO: implement")
}

// Order returns every service in one valid deployment order: the waves of
// Waves, concatenated. If the graph has a cycle, Order returns nil and the
// same *CycleError as Waves.
func (g *Graph) Order() ([]string, error) {
	panic("TODO: implement")
}

// Dependents returns every service that depends on service, directly or
// through other services: what breaks when service goes down. The result is
// sorted, has no duplicates and never contains service itself.
//
// For a service that is not in the graph, Dependents returns nil and an
// error that wraps ErrUnknownService and contains the service name.
func (g *Graph) Dependents(service string) ([]string, error) {
	panic("TODO: implement")
}

// Why returns the shortest dependency chain [from, ..., to], in which each
// service depends on the next one, and true. If there are several shortest
// chains, it returns the lexicographically first one. Why(s, s) for a known s
// returns [s] and true.
//
// If from or to is not in the graph, or from does not depend on to directly
// or indirectly, Why returns nil and false.
func (g *Graph) Why(from, to string) ([]string, bool) {
	panic("TODO: implement")
}
