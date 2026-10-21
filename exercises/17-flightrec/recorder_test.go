package flightrec_test

import (
	"fmt"
	"slices"
	"testing"

	flightrec "github.com/yornik/go-practice/exercises/17-flightrec"
)

func TestRecorderKeys(t *testing.T) {
	defer guard(t)()
	rec := flightrec.NewRecorder[string, int](3)
	if got := rec.Len(); got != 0 {
		t.Errorf("NewRecorder(3): Len() = %d, want 0", got)
	}
	if got := rec.Dump("vl-gpu-0412"); got != nil {
		t.Errorf("NewRecorder(3): Dump(%q) = %#v, want nil", "vl-gpu-0412", got)
	}

	// 2 keys, added in turns. vl-gpu-0412 gets 5 values and wraps; the 2
	// values of vl-gpu-0007 must survive that.
	rec.Add("vl-gpu-0412", 1)
	rec.Add("vl-gpu-0007", 10)
	rec.Add("vl-gpu-0412", 2)
	rec.Add("vl-gpu-0412", 3)
	rec.Add("vl-gpu-0007", 11)
	rec.Add("vl-gpu-0412", 4)
	rec.Add("vl-gpu-0412", 5)
	const what = "NewRecorder(3) after Add of 1 to 5 for vl-gpu-0412 and 10, 11 for vl-gpu-0007"

	if got, want := rec.Dump("vl-gpu-0412"), []int{3, 4, 5}; !slices.Equal(got, want) {
		t.Errorf("%s: Dump(%q) = %v, want %v", what, "vl-gpu-0412", got, want)
	}
	if got, want := rec.Dump("vl-gpu-0007"), []int{10, 11}; !slices.Equal(got, want) {
		t.Errorf("%s: Dump(%q) = %v, want %v", what, "vl-gpu-0007", got, want)
	}
	if got := rec.Dump("canary-h3-01"); got != nil {
		t.Errorf("%s: Dump(%q) = %#v, want nil (a key that was never added)", what, "canary-h3-01", got)
	}
	if got := rec.Len(); got != 2 {
		t.Errorf("%s: Len() = %d, want 2", what, got)
	}
}

func TestRecorderStructKey(t *testing.T) {
	defer guard(t)()
	type boot struct {
		Node string
		Boot int
	}
	rec := flightrec.NewRecorder[boot, string](4)
	rec.Add(boot{"vl-gpu-0412", 1}, "driver loaded")
	rec.Add(boot{"vl-gpu-0412", 1}, "ecc error")
	rec.Add(boot{"vl-gpu-0412", 2}, "driver loaded")

	// A key built anew with the same field values is the same key.
	key := boot{Node: "vl-gpu-" + "0412", Boot: 1}
	if got, want := rec.Dump(key), []string{"driver loaded", "ecc error"}; !slices.Equal(got, want) {
		t.Errorf("Dump(%+v) = %q, want %q", key, got, want)
	}
	if got, want := rec.Dump(boot{"vl-gpu-0412", 2}), []string{"driver loaded"}; !slices.Equal(got, want) {
		t.Errorf("Dump(%+v) = %q, want %q", boot{"vl-gpu-0412", 2}, got, want)
	}
	if got := rec.Dump(boot{"vl-gpu-0412", 3}); got != nil {
		t.Errorf("Dump(%+v) = %#v, want nil", boot{"vl-gpu-0412", 3}, got)
	}
	if got := rec.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2 (boot 1 and boot 2 of vl-gpu-0412)", got)
	}
}

func TestRecorderForget(t *testing.T) {
	defer guard(t)()
	rec := flightrec.NewRecorder[string, int](2)
	rec.Add("vl-gpu-0412", 1)
	rec.Add("vl-gpu-0412", 2)
	rec.Add("vl-gpu-0412", 3)
	rec.Add("vl-gpu-0007", 10)

	if !rec.Forget("vl-gpu-0412") {
		t.Errorf("Forget(%q) = false, want true (the key holds values)", "vl-gpu-0412")
	}
	if got := rec.Len(); got != 1 {
		t.Errorf("after Forget(%q): Len() = %d, want 1", "vl-gpu-0412", got)
	}
	if got := rec.Dump("vl-gpu-0412"); got != nil {
		t.Errorf("after Forget(%q): Dump(%q) = %#v, want nil", "vl-gpu-0412", "vl-gpu-0412", got)
	}
	if got, want := rec.Dump("vl-gpu-0007"), []int{10}; !slices.Equal(got, want) {
		t.Errorf("after Forget(%q): Dump(%q) = %v, want %v", "vl-gpu-0412", "vl-gpu-0007", got, want)
	}
	if rec.Forget("vl-gpu-0412") {
		t.Errorf("second Forget(%q) = true, want false", "vl-gpu-0412")
	}
	if rec.Forget("canary-h3-01") {
		t.Errorf("Forget(%q) = true, want false (a key that was never added)", "canary-h3-01")
	}
	if got := rec.Len(); got != 1 {
		t.Errorf("after Forget of unknown keys: Len() = %d, want 1", got)
	}

	// After Forget, the key starts with an empty ring.
	rec.Add("vl-gpu-0412", 4)
	if got, want := rec.Dump("vl-gpu-0412"), []int{4}; !slices.Equal(got, want) {
		t.Errorf("after Forget(%q) and Add(%q, 4): Dump(%q) = %v, want %v", "vl-gpu-0412", "vl-gpu-0412", "vl-gpu-0412", got, want)
	}
	if got := rec.Len(); got != 2 {
		t.Errorf("after Forget(%q) and Add(%q, 4): Len() = %d, want 2", "vl-gpu-0412", "vl-gpu-0412", got)
	}
}

func TestRecorderCopies(t *testing.T) {
	defer guard(t)()
	rec := flightrec.NewRecorder[string, int](3)
	rec.Add("vl-gpu-0412", 1)
	rec.Add("vl-gpu-0412", 2)

	got := rec.Dump("vl-gpu-0412")
	if !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("Dump(%q) = %v, want [1 2]", "vl-gpu-0412", got)
	}
	got[0] = -1
	got = append(got, 100)
	rec.Add("vl-gpu-0412", 3)
	if !slices.Equal(got, []int{-1, 2, 100}) {
		t.Errorf("s := Dump(%q); s[0] = -1; s = append(s, 100); Add(%q, 3): s = %v, want [-1 2 100]", "vl-gpu-0412", "vl-gpu-0412", got)
	}
	if again, want := rec.Dump("vl-gpu-0412"), []int{1, 2, 3}; !slices.Equal(again, want) {
		t.Errorf("s := Dump(%q); s[0] = -1; s = append(s, 100); Add(%q, 3): Dump(%q) = %v, want %v", "vl-gpu-0412", "vl-gpu-0412", "vl-gpu-0412", again, want)
	}
}

func TestRecorderPanics(t *testing.T) {
	defer guard(t)()
	if rec := flightrec.NewRecorder[string, int](1); rec == nil || rec.Len() != 0 {
		t.Fatalf("NewRecorder(1) = %v, want an empty recorder", rec)
	}
	for _, size := range []int{0, -1} {
		if v := panicValue(func() { flightrec.NewRecorder[string, int](size) }); v == nil {
			t.Errorf("NewRecorder(%d) did not panic, want a panic", size)
		}
	}
}

func ExampleRecorder() {
	type entry struct {
		Seq int
		Msg string
	}
	rec := flightrec.NewRecorder[string, entry](2)
	rec.Add("vl-gpu-0412", entry{1, "boot"})
	rec.Add("vl-gpu-0007", entry{1, "boot"})
	rec.Add("vl-gpu-0412", entry{2, "infer"})
	rec.Add("vl-gpu-0412", entry{3, "crash"})

	fmt.Println(rec.Len())
	fmt.Printf("%+v\n", rec.Dump("vl-gpu-0412"))
	fmt.Printf("%+v\n", rec.Dump("vl-gpu-0007"))
	fmt.Println(rec.Dump("canary-h3-01") == nil)
	// Output:
	// 2
	// [{Seq:2 Msg:infer} {Seq:3 Msg:crash}]
	// [{Seq:1 Msg:boot}]
	// true
}
