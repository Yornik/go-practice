package leaderelection_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	leaderelection "github.com/yornik/go-practice/exercises/48-leaderelection"
)

func TestFencedResourceZeroValue(t *testing.T) {
	var r leaderelection.FencedResource // ready to use without a constructor
	if v, token := r.Read(); v != "" || token != 0 {
		t.Errorf("Read() on a new resource = %q, %d; want \"\", 0", v, token)
	}
	if err := r.Write(1, "replicas=3"); err != nil {
		t.Errorf("Write(1, %q) on a new resource = %v, want nil", "replicas=3", err)
	}
	if v, token := r.Read(); v != "replicas=3" || token != 1 {
		t.Errorf("Read() = %q, %d; want %q, 1", v, token, "replicas=3")
	}
}

func TestFencedResourceRejectsStaleTokens(t *testing.T) {
	var r leaderelection.FencedResource
	steps := []struct {
		token     uint64
		v         string
		stale     bool   // Write must fail with ErrStaleToken
		wantV     string // Read after the write
		wantToken uint64
	}{
		{1, "a: replicas=3", false, "a: replicas=3", 1},
		{1, "a: replicas=4", false, "a: replicas=4", 1}, // the same leader writes again
		{3, "c: replicas=5", false, "c: replicas=5", 3}, // tokens may skip numbers
		{2, "b: replicas=1", true, "c: replicas=5", 3},  // b was replaced by c
		{1, "a: replicas=9", true, "c: replicas=5", 3},  // a woke up from a long pause
		{3, "c: replicas=6", false, "c: replicas=6", 3},
		{4, "d: replicas=2", false, "d: replicas=2", 4},
	}
	for i, s := range steps {
		err := r.Write(s.token, s.v)
		switch {
		case s.stale && !errors.Is(err, leaderelection.ErrStaleToken):
			t.Errorf("step %d: Write(%d, %q) = %v, want ErrStaleToken (a higher token was seen)", i+1, s.token, s.v, err)
		case !s.stale && err != nil:
			t.Errorf("step %d: Write(%d, %q) = %v, want nil", i+1, s.token, s.v, err)
		}
		if v, token := r.Read(); v != s.wantV || token != s.wantToken {
			t.Errorf("step %d: after Write(%d, %q): Read() = %q, %d; want %q, %d", i+1, s.token, s.v, v, token, s.wantV, s.wantToken)
		}
	}
}

func TestFencedResourceConcurrentWrites(t *testing.T) {
	// 8 writers each write the tokens 1 to 5000 in order, at the same time,
	// while a reader watches. Writers that keep pace pass the check together
	// all the time, so if checking the token and storing the value are two
	// steps, a lower token sooner or later lands after a higher one. Then the
	// highest token goes down, and lower tokens are accepted after a higher
	// one was. At the end the token must be 5000, with a value written with
	// 5000.
	var r leaderelection.FencedResource
	const writers, maxToken = 8, 5000
	var (
		wg          sync.WaitGroup
		mu          sync.Mutex
		problems    []string
		maxAccepted atomic.Uint64 // the highest token whose Write returned nil
	)
	report := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		if len(problems) < 5 {
			problems = append(problems, fmt.Sprintf(format, args...))
		}
	}
	stop := make(chan struct{})
	readerDone := make(chan struct{})
	go func() { // the reader
		defer close(readerDone)
		defer func() {
			if v := recover(); v != nil {
				report("Read panicked: %v", v)
			}
		}()
		var last uint64
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, token := r.Read(); token < last {
				report("Read() returned token %d after token %d: the highest token accepted went down", token, last)
				return
			} else {
				last = token
			}
		}
	}()
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					report("writer %d panicked: %v", w, v)
				}
			}()
			for token := uint64(1); token <= maxToken; token++ {
				before := maxAccepted.Load() // a Write that already succeeded
				err := r.Write(token, fmt.Sprintf("w%d@%d", w, token))
				switch {
				case err == nil && token < before:
					report("Write(%d, ...) = nil after Write(%d, ...) had succeeded: a lower token got in", token, before)
				case err == nil:
					for { // maxAccepted = max(maxAccepted, token)
						cur := maxAccepted.Load()
						if token <= cur || maxAccepted.CompareAndSwap(cur, token) {
							break
						}
					}
				case token == maxToken || !errors.Is(err, leaderelection.ErrStaleToken):
					report("Write(%d, ...) = %v, want nil or ErrStaleToken, and nil for the highest token %d", token, err, maxToken)
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-readerDone
	for _, p := range problems {
		t.Error(p)
	}
	v, token := r.Read()
	if token != maxToken || !strings.HasSuffix(v, fmt.Sprintf("@%d", maxToken)) {
		t.Errorf("Read() after all writes = %q, %d; want a value written with token %d, and token %d", v, token, maxToken, maxToken)
	}
}

func ExampleFencedResource() {
	var replicas leaderelection.FencedResource
	fmt.Println(replicas.Write(1, "web=3")) // leader a, epoch 1
	fmt.Println(replicas.Write(2, "web=5")) // b took over, epoch 2
	err := replicas.Write(1, "web=4")       // a wakes up from a long pause
	fmt.Println(errors.Is(err, leaderelection.ErrStaleToken))
	fmt.Println(replicas.Read())
	// Output:
	// <nil>
	// <nil>
	// true
	// web=5 2
}
