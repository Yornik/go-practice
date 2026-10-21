package pagenoise_test

import (
	"maps"
	"testing"

	pagenoise "github.com/yornik/go-practice/exercises/03-pagenoise"
)

func TestCountPairs(t *testing.T) {
	tests := []struct {
		name  string
		pages []pagenoise.Page
		want  map[pagenoise.Pair]int
	}{
		{"nil input", nil, map[pagenoise.Pair]int{}},
		{"empty input", []pagenoise.Page{}, map[pagenoise.Pair]int{}},
		{
			"1 page",
			[]pagenoise.Page{day("FeederOffline", "hq-window-01")},
			map[pagenoise.Pair]int{{Alert: "FeederOffline", Source: "hq-window-01"}: 1},
		},
		{
			"2 alerts from 1 source and 1 alert from 2 sources are 3 keys",
			[]pagenoise.Page{
				night("FeederOffline", "hq-window-01"),
				day("SeedSensorStuck", "hq-window-01"),
				day("FeederOffline", "hq-window-01"),
				night("PhotoStoreDiskFull", "photo-store-2"),
				day("PhotoStoreDiskFull", "photo-store-3"),
				day("PhotoStoreDiskFull", "photo-store-2"),
				day("FeederOffline", "hq-window-01"),
			},
			map[pagenoise.Pair]int{
				{Alert: "FeederOffline", Source: "hq-window-01"}:       3,
				{Alert: "SeedSensorStuck", Source: "hq-window-01"}:     1,
				{Alert: "PhotoStoreDiskFull", Source: "photo-store-2"}: 2,
				{Alert: "PhotoStoreDiskFull", Source: "photo-store-3"}: 1,
			},
		},
		{
			"the time of the page does not matter",
			[]pagenoise.Page{
				at(3, 12, "PushQueueBacklog", "push-1"),
				at(15, 0, "PushQueueBacklog", "push-1"),
				{Day: "2026-09-30", Hour: 22, Minute: 30, Alert: "PushQueueBacklog", Source: "push-1"},
			},
			map[pagenoise.Pair]int{{Alert: "PushQueueBacklog", Source: "push-1"}: 3},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pagenoise.CountPairs(tc.pages)
			if got == nil {
				t.Fatalf("CountPairs of these pages:%s\n= nil, want a non-nil map", describe(tc.pages))
			}
			if !maps.Equal(got, tc.want) {
				t.Errorf("CountPairs of these pages:%s\n got  %v\n want %v", describe(tc.pages), got, tc.want)
			}
		})
	}
}
