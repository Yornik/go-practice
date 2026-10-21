package pagenoise_test

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"

	pagenoise "github.com/yornik/go-practice/exercises/03-pagenoise"
)

// at builds a page on 2026-09-23 at hour:minute.
func at(hour, minute int, alert, source string) pagenoise.Page {
	return pagenoise.Page{Day: "2026-09-23", Hour: hour, Minute: minute, Alert: alert, Source: source}
}

// day and night build a page at 12:00 and at 03:00.
func day(alert, source string) pagenoise.Page   { return at(12, 0, alert, source) }
func night(alert, source string) pagenoise.Page { return at(3, 0, alert, source) }

func equalCount(a, b pagenoise.AlertCount) bool {
	return a.Alert == b.Alert && a.Pages == b.Pages && a.Night == b.Night && slices.Equal(a.Sources, b.Sources)
}

// format prints 1 AlertCount per line, so that a diff is easy to read.
func format(counts []pagenoise.AlertCount) string {
	if len(counts) == 0 {
		return "  (none)"
	}
	var b strings.Builder
	for i, c := range counts {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "  %+v", c)
	}
	return b.String()
}

// describe prints the pages of a test input, 1 per line.
func describe(pages []pagenoise.Page) string {
	var b strings.Builder
	for _, p := range pages {
		fmt.Fprintf(&b, "\n  %s %02d:%02d %s %s", p.Day, p.Hour, p.Minute, p.Alert, p.Source)
	}
	return b.String()
}

func byAlert(a, b pagenoise.AlertCount) int { return cmp.Compare(a.Alert, b.Alert) }

// checkRankIgnoringOrder fails the test unless got has the same entries as
// want, in any order.
func checkRankIgnoringOrder(t *testing.T, pages []pagenoise.Page, got, want []pagenoise.AlertCount) {
	t.Helper()
	got = slices.SortedFunc(slices.Values(got), byAlert)
	want = slices.SortedFunc(slices.Values(want), byAlert)
	if !slices.EqualFunc(got, want, equalCount) {
		t.Errorf("Rank of these pages:%s\ngot (sorted by Alert for this step)\n%s\nwant\n%s",
			describe(pages), format(got), format(want))
	}
}

// checkRank fails the test unless got equals want, in the same order.
func checkRank(t *testing.T, pages []pagenoise.Page, got, want []pagenoise.AlertCount) {
	t.Helper()
	if !slices.EqualFunc(got, want, equalCount) {
		t.Errorf("Rank of these pages:%s\ngot\n%s\nwant\n%s", describe(pages), format(got), format(want))
	}
}

func TestRankCountsEmpty(t *testing.T) {
	for _, pages := range [][]pagenoise.Page{nil, {}} {
		if got := pagenoise.Rank(pages); len(got) != 0 {
			t.Errorf("Rank(%#v) = %+v, want an empty result", pages, got)
		}
	}
}

func TestRankCounts(t *testing.T) {
	tests := []struct {
		name  string
		pages []pagenoise.Page
		want  []pagenoise.AlertCount
	}{
		{
			"1 page by day",
			[]pagenoise.Page{day("AirflowLow", "canary-h3-01")},
			[]pagenoise.AlertCount{{Alert: "AirflowLow", Pages: 1, Night: 0, Sources: []string{"canary-h3-01"}}},
		},
		{
			"1 page at night",
			[]pagenoise.Page{night("AirflowLow", "canary-h3-01")},
			[]pagenoise.AlertCount{{Alert: "AirflowLow", Pages: 1, Night: 1, Sources: []string{"canary-h3-01"}}},
		},
		{
			"the same source 3 times is 1 source",
			[]pagenoise.Page{
				day("NotifyQueueBacklog", "notify-1"),
				night("NotifyQueueBacklog", "notify-1"),
				day("NotifyQueueBacklog", "notify-1"),
			},
			[]pagenoise.AlertCount{{Alert: "NotifyQueueBacklog", Pages: 3, Night: 1, Sources: []string{"notify-1"}}},
		},
		{
			"sources are distinct and sorted A to Z",
			[]pagenoise.Page{
				night("AirflowLow", "canary-h3-01"),
				day("AirflowLow", "vl-gpu-0451"),
				day("AirflowLow", "canary-h3-01"),
				night("AirflowLow", "vl-gpu-0412"),
				day("AirflowLow", "vl-gpu-0451"),
			},
			[]pagenoise.AlertCount{{
				Alert: "AirflowLow", Pages: 5, Night: 2,
				Sources: []string{"canary-h3-01", "vl-gpu-0412", "vl-gpu-0451"},
			}},
		},
		{
			"each alert counts its own pages and sources",
			[]pagenoise.Page{
				day("CheckpointDiskFull", "chkpt-store-3"),
				night("AirflowLow", "canary-h3-01"),
				night("CheckpointDiskFull", "chkpt-store-2"),
				day("CheckoutErrorRate", "checkout-1"),
				day("CheckpointDiskFull", "chkpt-store-2"),
			},
			[]pagenoise.AlertCount{
				{Alert: "CheckpointDiskFull", Pages: 3, Night: 1, Sources: []string{"chkpt-store-2", "chkpt-store-3"}},
				{Alert: "AirflowLow", Pages: 1, Night: 1, Sources: []string{"canary-h3-01"}},
				{Alert: "CheckoutErrorRate", Pages: 1, Night: 0, Sources: []string{"checkout-1"}},
			},
		},
		{
			"alerts and sources compare as written",
			[]pagenoise.Page{
				day("AirflowLow", "canary-h3-01"),
				day("airflowlow", "canary-h3-01"),
				day("AirflowLow", "CANARY-h3-01"),
			},
			[]pagenoise.AlertCount{
				{Alert: "AirflowLow", Pages: 2, Night: 0, Sources: []string{"CANARY-h3-01", "canary-h3-01"}},
				{Alert: "airflowlow", Pages: 1, Night: 0, Sources: []string{"canary-h3-01"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pagenoise.Rank(tc.pages)
			checkRankIgnoringOrder(t, tc.pages, got, tc.want)
		})
	}
}

func TestRankOrderKeys(t *testing.T) {
	tests := []struct {
		name  string
		pages []pagenoise.Page
		want  []pagenoise.AlertCount
	}{
		{
			"more pages first, even with fewer at night",
			[]pagenoise.Page{
				night("AirflowLow", "canary-h3-01"),
				night("AirflowLow", "canary-h3-01"),
				day("NotifyQueueBacklog", "notify-1"),
				day("NotifyQueueBacklog", "notify-1"),
				day("NotifyQueueBacklog", "notify-1"),
			},
			[]pagenoise.AlertCount{
				{Alert: "NotifyQueueBacklog", Pages: 3, Night: 0, Sources: []string{"notify-1"}},
				{Alert: "AirflowLow", Pages: 2, Night: 2, Sources: []string{"canary-h3-01"}},
			},
		},
		{
			"equal pages: more at night first",
			[]pagenoise.Page{
				day("CheckoutErrorRate", "checkout-1"),
				day("CheckoutErrorRate", "checkout-1"),
				day("CheckpointDiskFull", "chkpt-store-2"),
				night("CheckpointDiskFull", "chkpt-store-2"),
			},
			[]pagenoise.AlertCount{
				{Alert: "CheckpointDiskFull", Pages: 2, Night: 1, Sources: []string{"chkpt-store-2"}},
				{Alert: "CheckoutErrorRate", Pages: 2, Night: 0, Sources: []string{"checkout-1"}},
			},
		},
		{
			"equal pages and night: alert A to Z",
			[]pagenoise.Page{
				night("FanSensorStuck", "vl-gpu-0437"),
				day("FanSensorStuck", "vl-gpu-0437"),
				day("CertificateExpiresSoon", "edge-1"),
				night("CertificateExpiresSoon", "edge-2"),
			},
			[]pagenoise.AlertCount{
				{Alert: "CertificateExpiresSoon", Pages: 2, Night: 1, Sources: []string{"edge-1", "edge-2"}},
				{Alert: "FanSensorStuck", Pages: 2, Night: 1, Sources: []string{"vl-gpu-0437"}},
			},
		},
		{
			"A to Z is byte order: upper case before lower case",
			[]pagenoise.Page{
				day("diskFull", "chkpt-store-1"),
				day("DiskFull", "chkpt-store-1"),
				day("Zombie", "chkpt-store-1"),
			},
			[]pagenoise.AlertCount{
				{Alert: "DiskFull", Pages: 1, Night: 0, Sources: []string{"chkpt-store-1"}},
				{Alert: "Zombie", Pages: 1, Night: 0, Sources: []string{"chkpt-store-1"}},
				{Alert: "diskFull", Pages: 1, Night: 0, Sources: []string{"chkpt-store-1"}},
			},
		},
		{
			"all 3 keys at once",
			[]pagenoise.Page{
				day("InferenceHighLatency", "infer-1"),
				night("DatabaseConnectionsHigh", "db-1"),
				day("NotifyQueueBacklog", "notify-1"),
				night("InferenceHighLatency", "infer-2"),
				night("NotifyQueueBacklog", "notify-1"),
				day("CheckoutErrorRate", "checkout-2"),
				day("NotifyQueueBacklog", "notify-1"),
				night("AirflowLow", "canary-h3-01"),
				day("CheckoutErrorRate", "checkout-1"),
			},
			[]pagenoise.AlertCount{
				{Alert: "NotifyQueueBacklog", Pages: 3, Night: 1, Sources: []string{"notify-1"}},
				{Alert: "InferenceHighLatency", Pages: 2, Night: 1, Sources: []string{"infer-1", "infer-2"}},
				{Alert: "CheckoutErrorRate", Pages: 2, Night: 0, Sources: []string{"checkout-1", "checkout-2"}},
				{Alert: "AirflowLow", Pages: 1, Night: 1, Sources: []string{"canary-h3-01"}},
				{Alert: "DatabaseConnectionsHigh", Pages: 1, Night: 1, Sources: []string{"db-1"}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pagenoise.Rank(tc.pages)
			checkRank(t, tc.pages, got, tc.want)
		})
	}
}

func TestRankOrderInputUnchanged(t *testing.T) {
	pages := []pagenoise.Page{
		day("FanSensorStuck", "vl-gpu-0437"),
		night("AirflowLow", "canary-h3-01"),
		day("CheckoutErrorRate", "checkout-1"),
		night("AirflowLow", "vl-gpu-0412"),
		day("NotifyQueueBacklog", "notify-1"),
	}
	before := slices.Clone(pages)
	pagenoise.Rank(pages)
	if !slices.Equal(pages, before) {
		t.Errorf("Rank changed its input\nbefore:%s\nafter:%s", describe(before), describe(pages))
	}
}

// TestRankOrderSameEveryRun calls Rank 20 times on 1 input with many ties
// and many sources. Map iteration order changes from run to run, so an
// order that depends on it shows up as a difference between 2 calls.
func TestRankOrderSameEveryRun(t *testing.T) {
	var pages []pagenoise.Page
	alerts := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel"}
	for i, alert := range alerts {
		// 2 alerts each with 1, 2, 3 and 4 pages; 1 page of each alert at night.
		for n := range i%4 + 1 {
			p := day(alert, fmt.Sprintf("source-%02d", (i*7+n*3)%10))
			if n == 0 {
				p.Hour = 23
			}
			pages = append(pages, p)
		}
	}
	first := pagenoise.Rank(pages)
	for run := 2; run <= 20; run++ {
		got := pagenoise.Rank(pages)
		if !slices.EqualFunc(got, first, equalCount) {
			t.Fatalf("Rank gave a different result on call %d than on call 1 for the same pages:%s\ncall 1\n%s\ncall %d\n%s",
				run, describe(pages), format(first), run, format(got))
		}
	}
	// Ties are decided by the alert, A to Z.
	want := []string{"Delta", "Hotel", "Charlie", "Golf", "Bravo", "Foxtrot", "Alpha", "Echo"}
	var gotAlerts []string
	for _, c := range first {
		gotAlerts = append(gotAlerts, c.Alert)
	}
	if !slices.Equal(gotAlerts, want) {
		t.Errorf("Rank order of alerts = %q, want %q\nfull result\n%s", gotAlerts, want, format(first))
	}
}

func TestRankNight(t *testing.T) {
	tests := []struct {
		hour, minute int
		night        bool
	}{
		{0, 0, true},
		{3, 12, true},
		{6, 59, true},
		{7, 0, false},
		{7, 1, false},
		{12, 0, false},
		{21, 59, false},
		{22, 0, true},
		{23, 59, true},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%02d:%02d", tc.hour, tc.minute), func(t *testing.T) {
			pages := []pagenoise.Page{at(tc.hour, tc.minute, "AirflowLow", "canary-h3-01")}
			wantNight := 0
			if tc.night {
				wantNight = 1
			}
			want := []pagenoise.AlertCount{{Alert: "AirflowLow", Pages: 1, Night: wantNight, Sources: []string{"canary-h3-01"}}}
			checkRank(t, pages, pagenoise.Rank(pages), want)
		})
	}
}
