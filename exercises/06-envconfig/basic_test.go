package envconfig_test

import (
	"errors"
	"testing"

	envconfig "github.com/yornik/go-practice/exercises/06-envconfig"
)

func TestTargetInvalid(t *testing.T) {
	type config struct{ Port int }
	var nilConfig *config
	port := 8080
	tests := []struct {
		name string
		dst  any
	}{
		{"nil", nil},
		{"struct value, not a pointer", config{}},
		{"nil pointer to a struct", nilConfig},
		{"pointer to an int", &port},
		{"pointer to a pointer to a struct", &nilConfig},
		{"map", map[string]string{"PORT": "8080"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := load(t, tc.dst, newEnv(map[string]string{"PORT": "9090"}))
			if !errors.Is(err, envconfig.ErrInvalidTarget) {
				t.Errorf("Load(%T) = %v, want an error wrapping ErrInvalidTarget", tc.dst, err)
			}
		})
	}
}

// basicConfig has only string and int fields, so you can pass TestBasic
// before you handle the other types.
type basicConfig struct {
	ListenAddr string // LISTEN_ADDR
	MaxConns   int    // MAX_CONNS
	HTTPPort   int    // HTTP_PORT
	Region     string `env:"AWS_REGION"` // the tag replaces the name
	Name       string `default:"checkout"`
	Workers    int    `default:"4" required:"true"` // a default satisfies required
	Internal   string `env:"-"`                     // never loaded
	token      string // unexported: never loaded, even though TOKEN is set
}

func TestBasicLoad(t *testing.T) {
	tests := []struct {
		name string
		dst  basicConfig // what dst holds before Load
		vars map[string]string
		want basicConfig
	}{
		{
			name: "every field from its variable",
			vars: map[string]string{
				"LISTEN_ADDR": ":8443", "MAX_CONNS": "100", "HTTP_PORT": "8080",
				"AWS_REGION": "eu-west-1", "NAME": "payments", "WORKERS": "16",
				// Traps: none of these may be read.
				"REGION": "us-east-1", "INTERNAL": "from-env", "-": "from-env", "TOKEN": "from-env",
			},
			want: basicConfig{ListenAddr: ":8443", MaxConns: 100, HTTPPort: 8080,
				Region: "eu-west-1", Name: "payments", Workers: 16},
		},
		{
			name: "defaults when nothing is set",
			vars: map[string]string{},
			want: basicConfig{Name: "checkout", Workers: 4},
		},
		{
			// A Helm template that renders `value: ""` sets the variable to
			// the empty string. That counts as not set.
			name: "an empty variable counts as not set",
			vars: map[string]string{"NAME": "", "WORKERS": "", "LISTEN_ADDR": "", "MAX_CONNS": ""},
			want: basicConfig{Name: "checkout", Workers: 4},
		},
		{
			name: "fields with no value keep what dst holds",
			dst: basicConfig{ListenAddr: ":9090", MaxConns: 50, Name: "set-in-code",
				Internal: "set-in-code", token: "set-in-code"},
			vars: map[string]string{"HTTP_PORT": "8080", "INTERNAL": "from-env", "TOKEN": "from-env"},
			// Name has a default, and a default replaces what dst holds.
			want: basicConfig{ListenAddr: ":9090", MaxConns: 50, HTTPPort: 8080, Name: "checkout",
				Workers: 4, Internal: "set-in-code", token: "set-in-code"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(tc.vars)
			got := tc.dst
			mustLoad(t, &got, e)
			if got != tc.want {
				t.Errorf("after Load:\n got %+v\nwant %+v\nkeys looked up: %q", got, tc.want, e.asked)
			}
		})
	}
}

func TestBasicNilLookupUsesProcessEnv(t *testing.T) {
	t.Setenv("GO_PRACTICE_06_LISTEN_ADDR", ":7070")
	var cfg struct {
		ListenAddr string `env:"GO_PRACTICE_06_LISTEN_ADDR"`
	}
	func() {
		defer catchPanic(t)
		if err := envconfig.Load(&cfg, nil); err != nil {
			t.Fatalf("Load(&cfg, nil) = %v, want nil", err)
		}
	}()
	if cfg.ListenAddr != ":7070" {
		t.Errorf("Load(&cfg, nil): ListenAddr = %q, want %q from the process environment (os.LookupEnv)", cfg.ListenAddr, ":7070")
	}
}
