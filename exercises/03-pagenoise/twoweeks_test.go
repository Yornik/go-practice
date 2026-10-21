package pagenoise_test

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	pagenoise "github.com/yornik/go-practice/exercises/03-pagenoise"
)

// readTwoWeeks parses every line of testdata/pages.log, 2 weeks of pages. It
// fails the test unless exactly lines 17 and 40 are malformed.
func readTwoWeeks(t *testing.T) []pagenoise.Page {
	t.Helper()
	data, err := os.ReadFile("testdata/pages.log")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 63 {
		t.Fatalf("testdata/pages.log has %d lines, want 63: restore the file", len(lines))
	}

	var pages []pagenoise.Page
	var bad []int
	for i, line := range lines {
		p, err := pagenoise.ParsePage(line)
		if err != nil {
			bad = append(bad, i+1)
			continue
		}
		pages = append(pages, p)
	}
	if want := []int{17, 40}; !slices.Equal(bad, want) {
		t.Fatalf("ParsePage failed on lines %v of testdata/pages.log, want exactly lines %v", bad, want)
	}
	return pages
}

func TestTwoWeeksRanking(t *testing.T) {
	pages := readTwoWeeks(t)
	got := pagenoise.Rank(pages)
	// Counted by hand: 61 pages, 19 of them at night.
	want := []pagenoise.AlertCount{
		{Alert: "FeederOffline", Pages: 18, Night: 9, Sources: []string{"feeder-0412", "feeder-1203", "hq-window-01"}},
		{Alert: "PhotoStoreDiskFull", Pages: 12, Night: 3, Sources: []string{"photo-store-2", "photo-store-3"}},
		{Alert: "PushQueueBacklog", Pages: 9, Night: 4, Sources: []string{"push-1"}},
		{Alert: "LabellerHighLatency", Pages: 9, Night: 2, Sources: []string{"labeller-1", "labeller-2"}},
		{Alert: "CheckoutErrorRate", Pages: 5, Night: 0, Sources: []string{"checkout-1", "checkout-2"}},
		{Alert: "DatabaseConnectionsHigh", Pages: 4, Night: 1, Sources: []string{"db-1"}},
		{Alert: "CertificateExpiresSoon", Pages: 2, Night: 0, Sources: []string{"edge-1", "edge-2"}},
		{Alert: "SeedSensorStuck", Pages: 2, Night: 0, Sources: []string{"feeder-0977"}},
	}
	if !slices.EqualFunc(got, want, equalCount) {
		t.Errorf("Rank of the 61 pages in testdata/pages.log\ngot\n%s\nwant\n%s", format(got), format(want))
	}
}

func TestTwoWeeksPairs(t *testing.T) {
	pages := readTwoWeeks(t)
	got := pagenoise.CountPairs(pages)
	want := map[pagenoise.Pair]int{
		{Alert: "FeederOffline", Source: "hq-window-01"}:       11,
		{Alert: "FeederOffline", Source: "feeder-0412"}:        4,
		{Alert: "FeederOffline", Source: "feeder-1203"}:        3,
		{Alert: "PhotoStoreDiskFull", Source: "photo-store-2"}: 8,
		{Alert: "PhotoStoreDiskFull", Source: "photo-store-3"}: 4,
		{Alert: "PushQueueBacklog", Source: "push-1"}:          9,
		{Alert: "LabellerHighLatency", Source: "labeller-1"}:   5,
		{Alert: "LabellerHighLatency", Source: "labeller-2"}:   4,
		{Alert: "CheckoutErrorRate", Source: "checkout-1"}:     3,
		{Alert: "CheckoutErrorRate", Source: "checkout-2"}:     2,
		{Alert: "DatabaseConnectionsHigh", Source: "db-1"}:     4,
		{Alert: "CertificateExpiresSoon", Source: "edge-1"}:    1,
		{Alert: "CertificateExpiresSoon", Source: "edge-2"}:    1,
		{Alert: "SeedSensorStuck", Source: "feeder-0977"}:      2,
	}
	if !maps.Equal(got, want) {
		t.Errorf("CountPairs of the 61 pages in testdata/pages.log\n got  %v\n want %v", got, want)
	}
}

func ExampleRank() {
	lines := []string{
		"2026-09-23 03:12 FeederOffline hq-window-01",
		"2026-09-23 07:50 PushQueueBacklog push-1",
		"2026-09-23 14:05 PhotoStoreDiskFull photo-store-2",
		"2026-09-23 22:00 FeederOffline feeder-0412",
		"2026-09-24 06:30 PushQueueBacklog push-1",
		"2026-09-24 11:20 PhotoStoreDiskFull photo-store-3",
		"2026-09-24 23:41 FeederOffline hq-window-01",
	}
	var pages []pagenoise.Page
	for _, line := range lines {
		p, err := pagenoise.ParsePage(line)
		if err != nil {
			fmt.Println(err)
			continue
		}
		pages = append(pages, p)
	}
	for _, c := range pagenoise.Rank(pages) {
		fmt.Println(c.Alert, c.Pages, c.Night, c.Sources)
	}
	// Output:
	// FeederOffline 3 3 [feeder-0412 hq-window-01]
	// PushQueueBacklog 2 1 [push-1]
	// PhotoStoreDiskFull 2 0 [photo-store-2 photo-store-3]
}
