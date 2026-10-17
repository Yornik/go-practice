package envconfig_test

import (
	"testing"

	envconfig "github.com/yornik/go-practice/exercises/06-envconfig"
)

// fakeEnv is an environment for one test. Its lookup method has the signature
// of os.LookupEnv, so it can be passed to Load. It records every key Load asks
// for; the tests print that list when a value is wrong, so you can see which
// variable names your Load computed.
type fakeEnv struct {
	vars  map[string]string
	asked []string
}

func newEnv(vars map[string]string) *fakeEnv {
	return &fakeEnv{vars: vars}
}

func (e *fakeEnv) lookup(key string) (string, bool) {
	e.asked = append(e.asked, key)
	v, ok := e.vars[key]
	return v, ok
}

// load calls envconfig.Load(dst, e.lookup). If Load panics (a reflect call on
// the wrong kind, a Set on an unexported field, a "TODO: implement"), the test
// fails with the panic message instead of killing the whole test binary.
func load(t *testing.T, dst any, e *fakeEnv) error {
	t.Helper()
	defer func() {
		if v := recover(); v != nil {
			t.Fatalf("Load panicked: %v\nkeys looked up before the panic: %q", v, e.asked)
		}
	}()
	return envconfig.Load(dst, e.lookup)
}

// catchPanic turns a panic in the test's goroutine, such as the
// "TODO: implement" of a method you have not written yet, into a test failure.
// Use it as: defer catchPanic(t).
func catchPanic(t *testing.T) {
	t.Helper()
	if v := recover(); v != nil {
		t.Fatalf("panicked: %v", v)
	}
}

// mustLoad is load for cases that must succeed.
func mustLoad(t *testing.T, dst any, e *fakeEnv) {
	t.Helper()
	if err := load(t, dst, e); err != nil {
		t.Fatalf("Load returned an error, want nil:\n%v\nkeys looked up: %q", err, e.asked)
	}
}

// fieldErrors returns the *FieldError values inside err, in order. It looks
// through errors.Join, also when joins are nested. It reports an error for
// anything in err that is neither a join nor a *FieldError.
func fieldErrors(t *testing.T, err error) []*envconfig.FieldError {
	t.Helper()
	var out []*envconfig.FieldError
	var walk func(error)
	walk = func(e error) {
		if fe, ok := e.(*envconfig.FieldError); ok {
			out = append(out, fe)
			return
		}
		if j, ok := e.(interface{ Unwrap() []error }); ok {
			for _, inner := range j.Unwrap() {
				walk(inner)
			}
			return
		}
		t.Errorf("Load returned an error %q of type %T: want only *envconfig.FieldError values, joined with errors.Join", e, e)
	}
	if err != nil {
		walk(err)
	}
	return out
}

// fieldErrorSummary is what most tests compare: everything in a FieldError
// except Err, which the tests check with errors.Is.
type fieldErrorSummary struct {
	Field, Key, Value string
	Secret            bool
}

func summarize(errs []*envconfig.FieldError) []fieldErrorSummary {
	out := make([]fieldErrorSummary, len(errs))
	for i, fe := range errs {
		out[i] = fieldErrorSummary{Field: fe.Field, Key: fe.Key, Value: fe.Value, Secret: fe.Secret}
	}
	return out
}
