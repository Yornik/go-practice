package envconfig_test

import (
	"errors"
	"log/slog"
	"net/netip"
	"reflect"
	"slices"
	"testing"
	"time"

	envconfig "github.com/yornik/go-practice/exercises/31-envconfig"
)

// region is a named type with kind string and no methods.
type region string

// typesConfig has one field per supported value type.
type typesConfig struct {
	Region           region
	Debug            bool
	Retries          int
	MaxBodyBytes     int64
	Shards           uint
	Port             uint16
	SampleRatio      float64
	Timeout          time.Duration // an int64 underneath, parsed with time.ParseDuration
	LogLevel         slog.Level    // an int underneath, parses itself (UnmarshalText)
	Bind             netip.Addr    // a struct, parses itself
	MaintenanceStart time.Time     // a struct, parses itself (RFC 3339)
}

func TestTypesValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		get   func(c typesConfig) any
		want  any
	}{
		{"named string type", "REGION", "eu-west-1", func(c typesConfig) any { return c.Region }, region("eu-west-1")},
		{"bool true", "DEBUG", "true", func(c typesConfig) any { return c.Debug }, true},
		{"bool 1", "DEBUG", "1", func(c typesConfig) any { return c.Debug }, true},
		{"bool FALSE", "DEBUG", "FALSE", func(c typesConfig) any { return c.Debug }, false},
		{"int", "RETRIES", "3", func(c typesConfig) any { return c.Retries }, 3},
		{"negative int", "RETRIES", "-1", func(c typesConfig) any { return c.Retries }, -1},
		{"int64", "MAX_BODY_BYTES", "10737418240", func(c typesConfig) any { return c.MaxBodyBytes }, int64(10 << 30)},
		{"uint", "SHARDS", "12", func(c typesConfig) any { return c.Shards }, uint(12)},
		{"uint16", "PORT", "8443", func(c typesConfig) any { return c.Port }, uint16(8443)},
		{"float64", "SAMPLE_RATIO", "0.25", func(c typesConfig) any { return c.SampleRatio }, 0.25},
		{"time.Duration", "TIMEOUT", "1m30s", func(c typesConfig) any { return c.Timeout }, 90 * time.Second},
		{"time.Duration in ms", "TIMEOUT", "250ms", func(c typesConfig) any { return c.Timeout }, 250 * time.Millisecond},
		{"slog.Level", "LOG_LEVEL", "warn", func(c typesConfig) any { return c.LogLevel }, slog.LevelWarn},
		{"slog.Level, any case", "LOG_LEVEL", "ERROR", func(c typesConfig) any { return c.LogLevel }, slog.LevelError},
		{"netip.Addr v4", "BIND", "10.0.0.7", func(c typesConfig) any { return c.Bind }, netip.MustParseAddr("10.0.0.7")},
		{"netip.Addr v6", "BIND", "::1", func(c typesConfig) any { return c.Bind }, netip.IPv6Loopback()},
		{"time.Time", "MAINTENANCE_START", "2026-10-17T22:00:00Z", func(c typesConfig) any { return c.MaintenanceStart },
			time.Date(2026, 10, 17, 22, 0, 0, 0, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(map[string]string{tc.key: tc.value})
			var cfg typesConfig
			mustLoad(t, &cfg, e)
			if got := tc.get(cfg); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s=%q: field = %v (%T), want %v (%T)\nkeys looked up: %q",
					tc.key, tc.value, got, got, tc.want, tc.want, e.asked)
			}
		})
	}
}

func TestTypesBadValues(t *testing.T) {
	tests := []struct {
		name       string
		field, key string
		value      string
	}{
		{"bool: yes is not a bool", "Debug", "DEBUG", "yes"},
		{"int: letters", "Retries", "RETRIES", "3x"},
		{"int: a fraction", "Retries", "RETRIES", "2.5"},
		{"uint: negative", "Shards", "SHARDS", "-1"},
		// An error, not 70000 % 65536 = 4464: a silent wrap is worse.
		{"uint16: does not fit", "Port", "PORT", "70000"},
		{"float64: decimal comma", "SampleRatio", "SAMPLE_RATIO", "0,25"},
		// A common mistake in a Deployment: seconds without a unit.
		{"time.Duration: no unit", "Timeout", "TIMEOUT", "30"},
		{"slog.Level: unknown name", "LogLevel", "LOG_LEVEL", "verbose"},
		{"netip.Addr: octet too big", "Bind", "BIND", "10.0.0.300"},
		{"time.Time: date only", "MaintenanceStart", "MAINTENANCE_START", "2026-10-17"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cfg typesConfig
			err := load(t, &cfg, newEnv(map[string]string{tc.key: tc.value}))
			errs := fieldErrors(t, err)
			want := []fieldErrorSummary{{Field: tc.field, Key: tc.key, Value: tc.value}}
			if !slices.Equal(summarize(errs), want) {
				t.Fatalf("%s=%q: Load returned field errors %+v, want %+v\nerror: %v", tc.key, tc.value, summarize(errs), want, err)
			}
			if errs[0].Err == nil {
				t.Errorf("%s=%q: FieldError.Err = nil, want the parse error", tc.key, tc.value)
			}
		})
	}
}

// listConfig has comma-separated lists.
type listConfig struct {
	Brokers []string
	Zones   []string `default:"eu-west-1a, eu-west-1b"`
}

func TestTypesStringSlice(t *testing.T) {
	tests := []struct {
		name string
		dst  listConfig
		vars map[string]string
		want listConfig
	}{
		{
			// ",," and ", ," are what is left after someone deletes an item.
			name: "items are trimmed, empty items dropped",
			vars: map[string]string{"BROKERS": " kafka-0:9092, kafka-1:9092,, ,kafka-2:9092 ,"},
			want: listConfig{Brokers: []string{"kafka-0:9092", "kafka-1:9092", "kafka-2:9092"},
				Zones: []string{"eu-west-1a", "eu-west-1b"}},
		},
		{
			name: "one item",
			vars: map[string]string{"BROKERS": "kafka-0:9092", "ZONES": "us-east-1a"},
			want: listConfig{Brokers: []string{"kafka-0:9092"}, Zones: []string{"us-east-1a"}},
		},
		{
			name: "unset: nil, or the default",
			vars: map[string]string{},
			want: listConfig{Zones: []string{"eu-west-1a", "eu-west-1b"}},
		},
		{
			name: "a value replaces the slice in dst, it does not append to it",
			dst:  listConfig{Brokers: []string{"old-0:9092", "old-1:9092"}},
			vars: map[string]string{"BROKERS": "new-0:9092"},
			want: listConfig{Brokers: []string{"new-0:9092"}, Zones: []string{"eu-west-1a", "eu-west-1b"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(tc.vars)
			got := tc.dst
			mustLoad(t, &got, e)
			if !slices.Equal(got.Brokers, tc.want.Brokers) || !slices.Equal(got.Zones, tc.want.Zones) {
				t.Errorf("after Load:\n got Brokers %q, Zones %q\nwant Brokers %q, Zones %q",
					got.Brokers, got.Zones, tc.want.Brokers, tc.want.Zones)
			}
			if tc.want.Brokers == nil && got.Brokers != nil {
				t.Errorf("Brokers = %#v, want nil: an unset variable must not create a slice", got.Brokers)
			}
		})
	}
}

// pointerConfig uses pointers to tell "not set" (nil) from a zero value.
type pointerConfig struct {
	Verbose  *bool
	Limit    *int
	Timeout  *time.Duration `default:"10s"`
	Upstream *netip.Addr
	Level    *slog.Level
}

func TestTypesPointers(t *testing.T) {
	t.Run("unset: nil, unless there is a default", func(t *testing.T) {
		var cfg pointerConfig
		mustLoad(t, &cfg, newEnv(map[string]string{}))
		if cfg.Verbose != nil || cfg.Limit != nil || cfg.Upstream != nil || cfg.Level != nil {
			t.Errorf("Verbose, Limit, Upstream, Level = %v, %v, %v, %v; want all nil", cfg.Verbose, cfg.Limit, cfg.Upstream, cfg.Level)
		}
		if cfg.Timeout == nil || *cfg.Timeout != 10*time.Second {
			t.Errorf("Timeout = %v, want a pointer to 10s (the default)", cfg.Timeout)
		}
	})

	t.Run("set: a pointer to the value, also for zero values", func(t *testing.T) {
		var cfg pointerConfig
		mustLoad(t, &cfg, newEnv(map[string]string{
			"VERBOSE": "false", "LIMIT": "0", "TIMEOUT": "2s", "UPSTREAM": "10.0.0.7", "LEVEL": "debug",
		}))
		if cfg.Verbose == nil || *cfg.Verbose {
			t.Errorf("VERBOSE=false: Verbose = %v, want a pointer to false", cfg.Verbose)
		}
		if cfg.Limit == nil || *cfg.Limit != 0 {
			t.Errorf("LIMIT=0: Limit = %v, want a pointer to 0", cfg.Limit)
		}
		if cfg.Timeout == nil || *cfg.Timeout != 2*time.Second {
			t.Errorf("TIMEOUT=2s: Timeout = %v, want a pointer to 2s", cfg.Timeout)
		}
		if cfg.Upstream == nil || *cfg.Upstream != netip.MustParseAddr("10.0.0.7") {
			t.Errorf("UPSTREAM=10.0.0.7: Upstream = %v, want a pointer to 10.0.0.7", cfg.Upstream)
		}
		if cfg.Level == nil || *cfg.Level != slog.LevelDebug {
			t.Errorf("LEVEL=debug: Level = %v, want a pointer to slog.LevelDebug", cfg.Level)
		}
	})

	t.Run("a pointer already in dst is kept when unset", func(t *testing.T) {
		limit := 5
		cfg := pointerConfig{Limit: &limit}
		mustLoad(t, &cfg, newEnv(map[string]string{}))
		if cfg.Limit != &limit {
			t.Errorf("Limit = %v, want the pointer that was already in dst", cfg.Limit)
		}
	})

	t.Run("a value never writes through a pointer already in dst", func(t *testing.T) {
		// The caller's variable may be shared, for example a package-level default.
		limit := 5
		cfg := pointerConfig{Limit: &limit}
		mustLoad(t, &cfg, newEnv(map[string]string{"LIMIT": "50"}))
		if cfg.Limit == nil || *cfg.Limit != 50 {
			t.Errorf("LIMIT=50: Limit = %v, want a pointer to 50", cfg.Limit)
		}
		if limit != 5 {
			t.Errorf("LIMIT=50 changed the variable the old pointer pointed to: it is %d, want 5 (allocate a new value)", limit)
		}
	})
}

// unsupportedConfig has field types that Load cannot fill.
type unsupportedConfig struct {
	Port    int
	Labels  map[string]string
	Weights []int
	Cache   map[string]string `env:"-"` // skipped, so not an error
	Logger  *slog.Logger      `env:"-"` // set in code, not from the env
}

func TestTypesUnsupported(t *testing.T) {
	for _, vars := range []map[string]string{
		{"PORT": "8080"}, // the unsupported variables are not even set: still an error
		{"PORT": "8080", "LABELS": "team=sre", "WEIGHTS": "1,2,3", "CACHE": "x", "LOGGER": "x"},
	} {
		logger := slog.Default()
		cfg := unsupportedConfig{Logger: logger}
		err := load(t, &cfg, newEnv(vars))
		errs := fieldErrors(t, err)
		want := []fieldErrorSummary{{Field: "Labels", Key: "LABELS"}, {Field: "Weights", Key: "WEIGHTS"}}
		if !slices.Equal(summarize(errs), want) {
			t.Fatalf("vars %v: Load returned field errors %+v, want %+v\nerror: %v", vars, summarize(errs), want, err)
		}
		for _, fe := range errs {
			if !errors.Is(fe, envconfig.ErrUnsupportedType) {
				t.Errorf("vars %v: FieldError for %s = %v, want it to wrap ErrUnsupportedType", vars, fe.Key, fe)
			}
		}
		if cfg.Logger != logger {
			t.Errorf("vars %v: Logger was changed, want the logger set in code (env:\"-\")", vars)
		}
	}
}
