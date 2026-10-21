package tableprint_test

import "testing"

func TestRenderBasic(t *testing.T) {
	type replicaSet struct {
		Name    string
		Desired int `table:"DESIRED REPLICAS"`
		Ready   int
	}
	type feeder struct {
		Name     string
		Firmware string `table:"FW"`
		token    string
		Owner    string `table:"-"`
		Battery  int    `table:"BATTERY%"`
	}
	type host struct {
		Name string
	}

	tests := []struct {
		name string
		rows any
		want string
	}{
		{
			name: "3 columns",
			rows: []pod{
				{"checkout-7f9c4", "Running", 0},
				{"orders-5d8b2", "CrashLoopBackOff", 12},
				{"payments-0", "Pending", 3},
			},
			want: "" +
				"NAME            STATUS            RESTARTS\n" +
				"checkout-7f9c4  Running           0\n" +
				"orders-5d8b2    CrashLoopBackOff  12\n" +
				"payments-0      Pending           3\n",
		},
		{
			name: "a header wider than every cell",
			rows: []replicaSet{{"checkout", 3, 3}},
			want: "" +
				"NAME      DESIRED REPLICAS  READY\n" +
				"checkout  3                 3\n",
		},
		{
			name: "tags, a hidden field and an unexported field",
			rows: []feeder{
				{"hq-window-01", "2.10.0", "t-1", "Ines", 87},
				{"feeder-0412", "2.9.3", "t-2", "Dot", 4},
			},
			want: "" +
				"NAME          FW      BATTERY%\n" +
				"hq-window-01  2.10.0  87\n" +
				"feeder-0412   2.9.3   4\n",
		},
		{
			name: "an empty string in a middle column is padded",
			rows: []pod{{"payments-0", "", 0}, {"orders-5d8b2", "Pending", 1}},
			want: "" +
				"NAME          STATUS   RESTARTS\n" +
				"payments-0             0\n" +
				"orders-5d8b2  Pending  1\n",
		},
		{
			name: "1 column is not padded",
			rows: []host{{"hq-window-01"}, {"photo-store-2"}, {"db"}},
			want: "" +
				"NAME\n" +
				"hq-window-01\n" +
				"photo-store-2\n" +
				"db\n",
		},
		{
			name: "an empty slice prints only the header row",
			rows: []pod{},
			want: "NAME  STATUS  RESTARTS\n",
		},
		{
			name: "a nil slice prints only the header row",
			rows: []pod(nil),
			want: "NAME  STATUS  RESTARTS\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			wantRender(t, tt.rows, tt.want)
		})
	}
}

func TestRenderWidth(t *testing.T) {
	type owner struct {
		Name string `table:"N"`
		Zone string
	}
	type sighting struct {
		Bird   string
		Feeder string
	}
	type city struct {
		City    string
		Feeders int
	}

	tests := []struct {
		name string
		rows any
		want string
	}{
		{
			// "zoë" is 4 bytes and 3 runes.
			name: "a 2-byte letter counts as 1",
			rows: []owner{{"zoë", "eu-1"}, {"bob", "eu-2"}},
			want: "" +
				"N    ZONE\n" +
				"zoë  eu-1\n" +
				"bob  eu-2\n",
		},
		{
			// The bird is 4 bytes and 1 rune. A terminal shows it 2 columns
			// wide, but the width counts runes.
			name: "an emoji counts as 1",
			rows: []sighting{{"\U0001F426 robin", "feeder-0412"}, {"great tit", "hq-window-01"}},
			want: "" +
				"BIRD       FEEDER\n" +
				"\U0001F426 robin    feeder-0412\n" +
				"great tit  hq-window-01\n",
		},
		{
			// "São Paulo" is 10 bytes and 9 runes, "Łódź" 7 bytes and 4 runes.
			name: "the widest cell has multi-byte letters",
			rows: []city{{"São Paulo", 12}, {"Zürich", 4}, {"Łódź", 3}},
			want: "" +
				"CITY       FEEDERS\n" +
				"São Paulo  12\n" +
				"Zürich     4\n" +
				"Łódź       3\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			wantRender(t, tt.rows, tt.want)
		})
	}
}
