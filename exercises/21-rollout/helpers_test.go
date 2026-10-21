package rollout_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	rollout "github.com/yornik/go-practice/exercises/21-rollout"
)

// timeout bounds every wait for Wait in these tests. A correct solution
// returns in microseconds; the timeout only catches a Wait that hangs.
const timeout = 5 * time.Second

// tick0 is the time value of the first tick. Wait does not use it.
var tick0 = time.Date(2026, 12, 7, 17, 0, 0, 0, time.UTC)

// Errors that the scripted polls return.
var (
	errRefused = errors.New("dial tcp 10.0.0.1:6443: connection refused")
	errTimeout = errors.New("the server was unable to return a response in the time allotted")
	errDenied  = errors.New("deployments.apps \"photo-api\" is forbidden")
)

var errNotScripted = errors.New("poller: Poll was called more often than the test expects")

// st is a short way to write a Status.
func st(desired, updated, ready int) rollout.Status {
	return rollout.Status{Desired: desired, Updated: updated, Ready: ready}
}

// result is what one scripted poll returns.
type result struct {
	status rollout.Status
	err    error
}

func (r result) String() string {
	if r.err != nil {
		return fmt.Sprintf("(%+v, %q)", r.status, r.err)
	}
	return fmt.Sprintf("%+v", r.status)
}

// ok is a successful poll; fail is a failed one.
func ok(desired, updated, ready int) result { return result{status: st(desired, updated, ready)} }
func fail(err error) result                 { return result{err: err} }

// poller is a scripted Options.Poll. Call number n (1-based) returns
// script[n-1]. A call past the end of the script returns errNotScripted. If
// hook is set, it runs at the start of every call, with the ctx that Wait
// passed and the call number. poller is safe for concurrent use.
type poller struct {
	script []result
	hook   func(ctx context.Context, n int)

	mu    sync.Mutex
	calls int
}

func (p *poller) Poll(ctx context.Context) (rollout.Status, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()
	if p.hook != nil {
		p.hook(ctx, n)
	}
	if n > len(p.script) {
		return rollout.Status{}, errNotScripted
	}
	r := p.script[n-1]
	return r.status, r.err
}

// Calls returns how many times Poll was called.
func (p *poller) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// describe formats the script for failure messages.
func (p *poller) describe() string {
	parts := make([]string, len(p.script))
	for i, r := range p.script {
		parts[i] = r.String()
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// outcome is what one call of Wait returned.
type outcome struct {
	status   rollout.Status
	err      error
	panicked any
}

// call is one call of Wait, running in its own goroutine. The test sends
// the ticks on an unbuffered channel, so every send that succeeds is a tick
// that Wait received.
type call struct {
	t     *testing.T
	ticks chan time.Time
	done  chan outcome // receives the outcome once
	out   *outcome     // set once Wait has returned
	sent  int          // ticks that Wait received
}

// start runs Wait(ctx, opt) in a new goroutine, with opt.Ticks set to the
// call's tick channel.
func start(t *testing.T, ctx context.Context, opt rollout.Options) *call {
	t.Helper()
	c := &call{t: t, ticks: make(chan time.Time), done: make(chan outcome, 1)}
	opt.Ticks = c.ticks
	c.run(ctx, opt)
	return c
}

// startRaw runs Wait(ctx, opt) in a new goroutine, with opt exactly as
// given. The call has no tick channel: do not call sendTicks.
func startRaw(t *testing.T, ctx context.Context, opt rollout.Options) *call {
	t.Helper()
	c := &call{t: t, done: make(chan outcome, 1)}
	c.run(ctx, opt)
	return c
}

func (c *call) run(ctx context.Context, opt rollout.Options) {
	go func() {
		var o outcome
		defer func() {
			o.panicked = recover()
			c.done <- o
		}()
		o.status, o.err = rollout.Wait(ctx, opt)
	}()
}

// sendTicks sends up to n ticks (n < 0: no limit) and stops early when Wait
// returns.
func (c *call) sendTicks(n int) {
	c.t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for n < 0 || c.sent < n {
		if c.out != nil {
			return
		}
		select {
		case c.ticks <- tick0.Add(time.Duration(c.sent+1) * 5 * time.Second):
			c.sent++
		case o := <-c.done:
			c.out = &o
		case <-deadline.C:
			c.hang("is it waiting for something other than a tick or ctx.Done()?")
		}
	}
}

// result waits for Wait to return and returns its results. It fails the
// test if Wait panicked or did not return in time.
func (c *call) result() (rollout.Status, error) {
	c.t.Helper()
	if c.out == nil {
		select {
		case o := <-c.done:
			c.out = &o
		case <-time.After(timeout):
			c.hang("it receives no more ticks: does it select on ctx.Done() while it waits for one, and does it end on a closed or nil Ticks?")
		}
	}
	if c.out.panicked != nil {
		c.t.Fatalf("Wait panicked: %v", c.out.panicked)
	}
	return c.out.status, c.out.err
}

// hang fails the test with the stacks of all goroutines.
func (c *call) hang(cause string) {
	c.t.Helper()
	var stacks bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&stacks, 1)
	c.t.Fatalf("Wait did not return within %v after %d ticks; %s\nGoroutines:\n%s", timeout, c.sent, cause, stacks.String())
}

// runWait calls Wait and sends ticks until Wait returns. It returns Wait's
// results and how many ticks Wait received.
func runWait(t *testing.T, ctx context.Context, opt rollout.Options) (rollout.Status, error, int) {
	t.Helper()
	c := start(t, ctx, opt)
	c.sendTicks(-1)
	s, err := c.result()
	return s, err, c.sent
}

// catchPanic turns a panic in the test's own goroutine into a test failure.
// Use it as: defer catchPanic(t).
func catchPanic(t *testing.T) {
	t.Helper()
	if v := recover(); v != nil {
		t.Fatalf("panicked: %v", v)
	}
}

// opts describes the options for failure messages.
func opts(o rollout.Options) string {
	return fmt.Sprintf("StallTicks %d, MaxErrors %d", o.StallTicks, o.MaxErrors)
}

// checkPolls fails the test unless Wait called Poll want times and received
// want-1 ticks: 1 poll right away, then 1 poll per tick.
func checkPolls(t *testing.T, p *poller, ticks, want int) {
	t.Helper()
	if got := p.Calls(); got != want {
		t.Errorf("Poll was called %d times, want %d", got, want)
	}
	if want > 0 && ticks != want-1 {
		t.Errorf("Wait received %d ticks for %d polls, want %d: 1 poll right away, then 1 poll per tick", ticks, p.Calls(), want-1)
	}
}

// waitCase is one table row: a script of polls, the options and what Wait
// must return. Wait runs with context.Background() and receives a tick
// whenever it waits for one.
type waitCase struct {
	name       string
	stallTicks int
	maxErrors  int
	script     []result
	want       rollout.Status
	wantErr    error // nil: Wait returns nil; otherwise errors.Is(err, wantErr)
	notErr     error // if set: errors.Is(err, notErr) must be false
	polls      int   // how many times Wait calls Poll
}

func (tc waitCase) run(t *testing.T) {
	t.Helper()
	defer catchPanic(t)
	p := &poller{script: tc.script}
	opt := rollout.Options{Poll: p.Poll, StallTicks: tc.stallTicks, MaxErrors: tc.maxErrors}
	got, err, ticks := runWait(t, context.Background(), opt)

	in := fmt.Sprintf("%s, polls %s", opts(opt), p.describe())
	switch {
	case tc.wantErr == nil && err != nil:
		t.Errorf("%s:\nWait = %+v, %v\nwant %+v, nil", in, got, err, tc.want)
	case tc.wantErr != nil && !errors.Is(err, tc.wantErr):
		t.Errorf("%s:\nWait = %+v, %v\nwant %+v and an error that wraps %q", in, got, err, tc.want, tc.wantErr)
	case tc.notErr != nil && errors.Is(err, tc.notErr):
		t.Errorf("%s:\nWait = %+v, %v\nwant an error that does not wrap %q", in, got, err, tc.notErr)
	case got != tc.want:
		t.Errorf("%s:\nWait = %+v, %v\nwant status %+v", in, got, err, tc.want)
	}
	checkPolls(t, p, ticks, tc.polls)
}
