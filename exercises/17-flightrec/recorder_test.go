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
	if got := rec.Dump("feeder-0412"); got != nil {
		t.Errorf("NewRecorder(3): Dump(%q) = %#v, want nil", "feeder-0412", got)
	}

	// 2 keys, added in turns. feeder-0412 gets 5 values and wraps; the 2
	// values of feeder-0007 must survive that.
	rec.Add("feeder-0412", 1)
	rec.Add("feeder-0007", 10)
	rec.Add("feeder-0412", 2)
	rec.Add("feeder-0412", 3)
	rec.Add("feeder-0007", 11)
	rec.Add("feeder-0412", 4)
	rec.Add("feeder-0412", 5)
	const what = "NewRecorder(3) after Add of 1 to 5 for feeder-0412 and 10, 11 for feeder-0007"

	if got, want := rec.Dump("feeder-0412"), []int{3, 4, 5}; !slices.Equal(got, want) {
		t.Errorf("%s: Dump(%q) = %v, want %v", what, "feeder-0412", got, want)
	}
	if got, want := rec.Dump("feeder-0007"), []int{10, 11}; !slices.Equal(got, want) {
		t.Errorf("%s: Dump(%q) = %v, want %v", what, "feeder-0007", got, want)
	}
	if got := rec.Dump("hq-window-01"); got != nil {
		t.Errorf("%s: Dump(%q) = %#v, want nil (a key that was never added)", what, "hq-window-01", got)
	}
	if got := rec.Len(); got != 2 {
		t.Errorf("%s: Len() = %d, want 2", what, got)
	}
}

func TestRecorderStructKey(t *testing.T) {
	defer guard(t)()
	type boot struct {
		Feeder string
		Boot   int
	}
	rec := flightrec.NewRecorder[boot, string](4)
	rec.Add(boot{"feeder-0412", 1}, "camera on")
	rec.Add(boot{"feeder-0412", 1}, "upload failed")
	rec.Add(boot{"feeder-0412", 2}, "camera on")

	// A key built anew with the same field values is the same key.
	key := boot{Feeder: "feeder-" + "0412", Boot: 1}
	if got, want := rec.Dump(key), []string{"camera on", "upload failed"}; !slices.Equal(got, want) {
		t.Errorf("Dump(%+v) = %q, want %q", key, got, want)
	}
	if got, want := rec.Dump(boot{"feeder-0412", 2}), []string{"camera on"}; !slices.Equal(got, want) {
		t.Errorf("Dump(%+v) = %q, want %q", boot{"feeder-0412", 2}, got, want)
	}
	if got := rec.Dump(boot{"feeder-0412", 3}); got != nil {
		t.Errorf("Dump(%+v) = %#v, want nil", boot{"feeder-0412", 3}, got)
	}
	if got := rec.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2 (boot 1 and boot 2 of feeder-0412)", got)
	}
}

func TestRecorderForget(t *testing.T) {
	defer guard(t)()
	rec := flightrec.NewRecorder[string, int](2)
	rec.Add("feeder-0412", 1)
	rec.Add("feeder-0412", 2)
	rec.Add("feeder-0412", 3)
	rec.Add("feeder-0007", 10)

	if !rec.Forget("feeder-0412") {
		t.Errorf("Forget(%q) = false, want true (the key holds values)", "feeder-0412")
	}
	if got := rec.Len(); got != 1 {
		t.Errorf("after Forget(%q): Len() = %d, want 1", "feeder-0412", got)
	}
	if got := rec.Dump("feeder-0412"); got != nil {
		t.Errorf("after Forget(%q): Dump(%q) = %#v, want nil", "feeder-0412", "feeder-0412", got)
	}
	if got, want := rec.Dump("feeder-0007"), []int{10}; !slices.Equal(got, want) {
		t.Errorf("after Forget(%q): Dump(%q) = %v, want %v", "feeder-0412", "feeder-0007", got, want)
	}
	if rec.Forget("feeder-0412") {
		t.Errorf("second Forget(%q) = true, want false", "feeder-0412")
	}
	if rec.Forget("hq-window-01") {
		t.Errorf("Forget(%q) = true, want false (a key that was never added)", "hq-window-01")
	}
	if got := rec.Len(); got != 1 {
		t.Errorf("after Forget of unknown keys: Len() = %d, want 1", got)
	}

	// After Forget, the key starts with an empty ring.
	rec.Add("feeder-0412", 4)
	if got, want := rec.Dump("feeder-0412"), []int{4}; !slices.Equal(got, want) {
		t.Errorf("after Forget(%q) and Add(%q, 4): Dump(%q) = %v, want %v", "feeder-0412", "feeder-0412", "feeder-0412", got, want)
	}
	if got := rec.Len(); got != 2 {
		t.Errorf("after Forget(%q) and Add(%q, 4): Len() = %d, want 2", "feeder-0412", "feeder-0412", got)
	}
}

func TestRecorderCopies(t *testing.T) {
	defer guard(t)()
	rec := flightrec.NewRecorder[string, int](3)
	rec.Add("feeder-0412", 1)
	rec.Add("feeder-0412", 2)

	got := rec.Dump("feeder-0412")
	if !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("Dump(%q) = %v, want [1 2]", "feeder-0412", got)
	}
	got[0] = -1
	got = append(got, 100)
	rec.Add("feeder-0412", 3)
	if !slices.Equal(got, []int{-1, 2, 100}) {
		t.Errorf("s := Dump(%q); s[0] = -1; s = append(s, 100); Add(%q, 3): s = %v, want [-1 2 100]", "feeder-0412", "feeder-0412", got)
	}
	if again, want := rec.Dump("feeder-0412"), []int{1, 2, 3}; !slices.Equal(again, want) {
		t.Errorf("s := Dump(%q); s[0] = -1; s = append(s, 100); Add(%q, 3): Dump(%q) = %v, want %v", "feeder-0412", "feeder-0412", "feeder-0412", again, want)
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
	rec.Add("feeder-0412", entry{1, "boot"})
	rec.Add("feeder-0007", entry{1, "boot"})
	rec.Add("feeder-0412", entry{2, "upload"})
	rec.Add("feeder-0412", entry{3, "crash"})

	fmt.Println(rec.Len())
	fmt.Printf("%+v\n", rec.Dump("feeder-0412"))
	fmt.Printf("%+v\n", rec.Dump("feeder-0007"))
	fmt.Println(rec.Dump("hq-window-01") == nil)
	// Output:
	// 2
	// [{Seq:2 Msg:upload} {Seq:3 Msg:crash}]
	// [{Seq:1 Msg:boot}]
	// true
}
