package singleflight_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	singleflight "github.com/yornik/go-practice/exercises/20-singleflight"
)

// crash is what the fake backend panics with in these tests.
const crash = "nil pointer in the cache backend"

// wantStackOfLoader checks that a PanicError's Stack shows the loader's
// method, where the panic happened.
func wantStackOfLoader(t *testing.T, pe *singleflight.PanicError) {
	t.Helper()
	if pe != nil && !bytes.Contains(pe.Stack, []byte("(*loader).fn")) {
		t.Errorf("PanicError.Stack does not mention (*loader).fn, the function that panicked. "+
			"Capture it with debug.Stack() inside the deferred function that recovers. Stack:\n%s", pe.Stack)
	}
}

func TestPanicInDo(t *testing.T) {
	// A panic in fn must reach every caller, in the caller's own goroutine.
	// The alternative is worse: callers that wait forever, or a crash in a
	// goroutine that nobody can recover.
	untilJoined(t, func(window time.Duration) int {
		var g group
		db := newLoader(t, "")
		db.panicValue = crash
		first := startDo(&g, "user:42", db.fn)
		db.waitStartedBy(t, "Do", first)

		viaChan := doChan(t, &g, "user:42", newQuick("unused").fn)
		lateCrash := "a later execution panicked"
		var others [5]<-chan outcome
		for i := range others {
			others[i] = startDo(&g, "user:42", func() (string, error) { panic(lateCrash) })
		}
		time.Sleep(window) // give the 5 callers time to reach Do

		db.release()
		pe := wantPanicError(t, "the first Do", await(t, "the first Do", first), crash)
		wantStackOfLoader(t, pe)

		r := receive(t, "DoChan joined to the execution that panicked", viaChan)
		var chanPE *singleflight.PanicError
		if !errors.As(r.Err, &chanPE) || chanPE.Value != crash || r.Val != "" {
			t.Errorf("DoChan Result = {Val:%q Err:%v}, want a zero Val and Err a *singleflight.PanicError with Value %q", r.Val, r.Err, crash)
		}

		joined := 0
		for i, ch := range others {
			o := await(t, fmt.Sprintf("Do number %d", i+2), ch)
			if pe, ok := o.panicked.(*singleflight.PanicError); ok && pe.Value == lateCrash {
				continue // it arrived after the execution had finished and ran its own fn
			}
			_ = wantPanicError(t, fmt.Sprintf("Do number %d, which joined the execution", i+2), o, crash)
			joined++
		}

		// The panic does not stick to the key: the next call runs fn again.
		fresh := newQuick("profile of user 42")
		want(t, "Do after the panic", do(t, &g, "user:42", fresh.fn), "profile of user 42", false)
		return joined
	})
}

func TestPanicInDoChan(t *testing.T) {
	// DoChan runs fn in a goroutine of its own. A panic there that is not
	// recovered would kill the whole test binary: then you see the panic
	// message and no test results at all.
	var g group
	db := newLoader(t, "")
	db.panicValue = crash
	first := doChan(t, &g, "user:42", db.fn)
	db.waitStarted(t)
	second := doChan(t, &g, "user:42", newQuick("unused").fn)

	db.release()
	for i, ch := range []<-chan singleflight.Result[string]{first, second} {
		r := receive(t, fmt.Sprintf("DoChan number %d", i+1), ch)
		var pe *singleflight.PanicError
		if !errors.As(r.Err, &pe) {
			t.Errorf("DoChan number %d: Result = %+v, want Err to be a *singleflight.PanicError", i+1, r)
			continue
		}
		if pe.Value != crash || r.Val != "" {
			t.Errorf("DoChan number %d: Result = {Val:%q Err:%v}, want a zero Val and a PanicError with Value %q", i+1, r.Val, r.Err, crash)
		}
		wantStackOfLoader(t, pe)
	}

	fresh := newQuick("profile of user 42")
	wantResult(t, "DoChan after the panic", receive(t, "DoChan", doChan(t, &g, "user:42", fresh.fn)), "profile of user 42", false)
}
