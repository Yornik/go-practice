package loglevels_test

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	loglevels "github.com/yornik/go-practice/exercises/01-loglevels"
)

// levelNames are the only keys CountLevels may return.
var levelNames = []string{"debug", "info", "warn", "error", "fatal", "unknown"}

// checkCounts fails the test unless got is non-nil and has the counts of
// want. A level with 0 lines may be absent from got or have the value 0.
func checkCounts(t *testing.T, call string, got, want map[string]int) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s = nil, want a non-nil map", call)
	}
	for k := range got {
		if !slices.Contains(levelNames, k) {
			t.Errorf("%s has the key %q, want only keys of %q", call, k, levelNames)
		}
	}
	nonZero := maps.Clone(got)
	maps.DeleteFunc(nonZero, func(_ string, n int) bool { return n == 0 })
	if !maps.Equal(nonZero, want) {
		t.Errorf("%s (entries with 0 left out)\n got  %v\n want %v", call, nonZero, want)
	}
}

func TestCountLevelsSmall(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  map[string]int
	}{
		{"nil input", nil, map[string]int{}},
		{"empty input", []string{}, map[string]int{}},
		{"only blank lines are skipped", []string{"", "   ", "\t", " \t \r"}, map[string]int{}},
		{"1 line", []string{"level=error"}, map[string]int{"error": 1}},
		{
			"3 spellings of error",
			[]string{"level=ERROR", "lvl=err", "severity=error"},
			map[string]int{"error": 3},
		},
		{
			"unknown lines are counted",
			[]string{"msg=boot", "level=verbose", "level=info", "", "level=", "=x"},
			map[string]int{"unknown": 4, "info": 1},
		},
		{
			"every level",
			[]string{"lvl=trace", "lvl=notice", "lvl=warning", "lvl=err", "lvl=emerg", "lvl=dbg", "msg=x"},
			map[string]int{"debug": 2, "info": 1, "warn": 1, "error": 1, "fatal": 1, "unknown": 1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := loglevels.CountLevels(tc.lines)
			checkCounts(t, fmt.Sprintf("CountLevels(%q)", tc.lines), got, tc.want)
		})
	}
}

// TestCountLevelsPhotoLog counts the levels in a log of the photo pipeline:
// 3 services with 3 ways to spell a level, and 3 blank lines.
func TestCountLevelsPhotoLog(t *testing.T) {
	data, err := os.ReadFile("testdata/photos.log")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")

	got := loglevels.CountLevels(lines)
	// Counted by hand. No line has exactly level=error.
	want := map[string]int{"debug": 3, "info": 5, "warn": 3, "error": 7, "fatal": 4, "unknown": 2}
	checkCounts(t, "CountLevels(lines of testdata/photos.log)", got, want)
}

func ExampleCountLevels() {
	lines := []string{
		"service=upload level=ERROR msg=disk_full",
		"feeder=feeder-0412 lvl=err msg=camera_timeout",
		"",
		"service=labeller severity=critical msg=model_oom",
		"service=labeller severity=Information msg=batch_done",
		"feeder=feeder-1203 msg=boot",
	}
	counts := loglevels.CountLevels(lines)
	for _, level := range []string{"debug", "info", "warn", "error", "fatal", "unknown"} {
		fmt.Println(level, counts[level])
	}
	// Output:
	// debug 0
	// info 1
	// warn 0
	// error 2
	// fatal 1
	// unknown 1
}
