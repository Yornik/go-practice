package tableprint_test

import (
	"math"
	"net/netip"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// oneCell returns a table with 1 column V and 1 row that holds v. Render
// prints it as "V\n" + the cell + "\n".
func oneCell[T any](v T) any {
	return []struct{ V T }{{v}}
}

type cellCase struct {
	name string
	rows any // made with oneCell
	want string
}

func runCellCases(t *testing.T, cases []cellCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer guard(t)()
			wantRender(t, tc.rows, "V\n"+tc.want+"\n")
		})
	}
}

// phase is a named string type without a String method.
type phase string

// exitCode is a named int type without a String method.
type exitCode int

// percent has String with a value receiver, so both percent and *percent
// implement fmt.Stringer.
type percent int

func (p percent) String() string { return strconv.Itoa(int(p)) + "%" }

// version is a struct with String with a value receiver.
type version struct{ Major, Minor, Patch int }

func (v version) String() string {
	return "v" + strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
}

// gauge has String with a pointer receiver, so only *gauge implements
// fmt.Stringer.
type gauge struct{ Value int }

func (g *gauge) String() string { return strconv.Itoa(g.Value) + " MB" }

// location is a struct without a String method.
type location struct {
	Region string
	Rack   int
}

var launchWeek = time.Date(2026, 11, 27, 21, 40, 0, 0, time.FixedZone("CET", 3600))

func TestRenderCellsBasicKinds(t *testing.T) {
	runCellCases(t, []cellCase{
		{"string", oneCell("chkpt-store-2"), "chkpt-store-2"},
		{"empty string", oneCell(""), ""},
		{"named string type", oneCell(phase("Degraded")), "Degraded"},
		{"bool true", oneCell(true), "true"},
		{"bool false", oneCell(false), "false"},
		{"int", oneCell(-3), "-3"},
		{"int8", oneCell(int8(-128)), "-128"},
		{"int16", oneCell(int16(-32768)), "-32768"},
		{"int32", oneCell(int32(70000)), "70000"},
		{"int64", oneCell(int64(math.MinInt64)), "-9223372036854775808"},
		{"uint", oneCell(uint(7)), "7"},
		{"uint8", oneCell(uint8(255)), "255"},
		{"uint16", oneCell(uint16(65535)), "65535"},
		{"uint32", oneCell(uint32(4294967295)), "4294967295"},
		{"uint64", oneCell(uint64(math.MaxUint64)), "18446744073709551615"},
		{"uintptr", oneCell(uintptr(42)), "42"},
		{"named int type", oneCell(exitCode(2)), "2"},
		{"float64 0.5", oneCell(0.5), "0.5"},
		{"float64 without a fraction", oneCell(2.0), "2"},
		{"float64 negative", oneCell(-0.25), "-0.25"},
		{"float64 large, no exponent", oneCell(1e21), "1000000000000000000000"},
		{"float64 small, no exponent", oneCell(1e-7), "0.0000001"},
		{"float32 0.5", oneCell(float32(0.5)), "0.5"},
		{"float32 0.1 with 32 bits", oneCell(float32(0.1)), "0.1"},
		{"float32 large with 32 bits", oneCell(float32(1e21)), "1000000000000000000000"},
	})
}

func TestRenderCellsStringer(t *testing.T) {
	runCellCases(t, []cellCase{
		{"time.Duration", oneCell(90 * time.Second), "1m30s"},
		{"time.Time in a fixed zone", oneCell(launchWeek), "2026-11-27 21:40:00 +0100 CET"},
		{"netip.Addr", oneCell(netip.MustParseAddr("192.0.2.10")), "192.0.2.10"},
		{"named int type with String", oneCell(percent(87)), "87%"},
		{"struct with String", oneCell(version{2, 10, 0}), "v2.10.0"},
		// Only *gauge has String. A gauge value does not, so rule 5 applies,
		// as it does in fmt.Sprint(gauge{7}).
		{"String with a pointer receiver does not count for a value", oneCell(gauge{7}), "{7}"},
	})
}

func TestRenderCellsFallback(t *testing.T) {
	runCellCases(t, []cellCase{
		{"nested struct", oneCell(location{"harbour-1", 3}), "{harbour-1 3}"},
		{"slice of strings", oneCell([]string{"checkout", "orders"}), "[checkout orders]"},
		{"nil slice of strings", oneCell([]string(nil)), "[]"},
		{"array of ints", oneCell([2]int{1, 2}), "[1 2]"},
		{"map", oneCell(map[string]int{"orders": 2, "checkout": 1}), "map[checkout:1 orders:2]"},
		{"complex128", oneCell(complex(1, 2)), "(1+2i)"},
	})
}

func TestRenderPointersFields(t *testing.T) {
	name := "chkpt-store-2"
	util := 87
	utilPtr := &util
	var nilInt *int
	at := launchWeek
	tick := 90 * time.Second
	endpoint, err := url.Parse("https://canary-h3-01.vesper-labs.example/healthz")
	if err != nil {
		t.Fatal(err)
	}
	pct := percent(87)

	runCellCases(t, []cellCase{
		{"nil pointer to a string", oneCell((*string)(nil)), "<none>"},
		{"pointer to a string", oneCell(&name), "chkpt-store-2"},
		{"nil pointer to an int", oneCell(nilInt), "<none>"},
		{"pointer to an int", oneCell(&util), "87"},
		{"pointer to a pointer to an int", oneCell(&utilPtr), "87"},
		{"pointer to a nil pointer to an int", oneCell(&nilInt), "<none>"},
		{"nil pointer to a time.Time", oneCell((*time.Time)(nil)), "<none>"},
		{"pointer to a time.Time", oneCell(&at), "2026-11-27 21:40:00 +0100 CET"},
		{"pointer to a time.Duration", oneCell(&tick), "1m30s"},
		{"pointer to a type with String on the value", oneCell(&pct), "87%"},
		// *url.URL is the usual way to hold a URL; String has a pointer
		// receiver.
		{"pointer to a type with String on the pointer", oneCell(endpoint), "https://canary-h3-01.vesper-labs.example/healthz"},
		{"pointer to a gauge", oneCell(&gauge{7}), "7 MB"},
		{"nil pointer to a type with String on the pointer", oneCell((*gauge)(nil)), "<none>"},
		{"nil *url.URL", oneCell((*url.URL)(nil)), "<none>"},
		// The cell of the struct it points to, not &{harbour-1 3}.
		{"pointer to a struct without String", oneCell(&location{"harbour-1", 3}), "{harbour-1 3}"},
	})
}

func TestRenderPointersRows(t *testing.T) {
	type withEmbeddedPointer struct {
		*Meta
		Phase string
	}
	type node struct {
		Name    string
		GPUUtil *int `table:"GPU UTIL"`
	}
	gpuUtil := 87

	tests := []struct {
		name string
		rows any
		want string
	}{
		{
			name: "slice of pointers to structs",
			rows: []*pod{
				{"checkout-7f9c4", "Running", 0},
				{"orders-5d8b2", "CrashLoopBackOff", 12},
			},
			want: "" +
				"NAME            STATUS            RESTARTS\n" +
				"checkout-7f9c4  Running           0\n" +
				"orders-5d8b2    CrashLoopBackOff  12\n",
		},
		{
			name: "empty slice of pointers prints only the header row",
			rows: []*pod{},
			want: "NAME  STATUS  RESTARTS\n",
		},
		{
			name: "pointer fields in pointer rows",
			rows: []*node{{"canary-h3-01", &gpuUtil}, {"vl-gpu-0412", nil}},
			want: "" +
				"NAME          GPU UTIL\n" +
				"canary-h3-01  87\n" +
				"vl-gpu-0412   <none>\n",
		},
		{
			name: "embedded pointer, nil and not nil",
			rows: []withEmbeddedPointer{{nil, "Pending"}, {&Meta{"orders-5d8b2", "eu-1"}, "Running"}},
			want: "" +
				"META                 PHASE\n" +
				"<none>               Pending\n" +
				"{orders-5d8b2 eu-1}  Running\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			wantRender(t, tt.rows, tt.want)
		})
	}
}
