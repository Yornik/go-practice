package envconfig_test

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	envconfig "github.com/yornik/go-practice/exercises/06-envconfig"
)

// serviceConfig is a typical service config with many mistakes in its
// environment at once.
type serviceConfig struct {
	Port    int           `default:"8080"`
	Timeout time.Duration `default:"5s"`
	Workers int           `default:"many"` // a bug in the code: the default does not parse
	DB      struct {
		URL      string `required:"true"`
		Password string `required:"true" secret:"true"`
	}
	Debug   bool
	Brokers []string `required:"true"`
}

func TestErrorsAllAtOnce(t *testing.T) {
	e := newEnv(map[string]string{
		"PORT":    "80a",
		"TIMEOUT": "30",
		"DEBUG":   "yes",
		"DB_URL":  "", // empty counts as not set
		"BROKERS": "kafka-0:9092",
	})
	var cfg serviceConfig
	err := load(t, &cfg, e)
	if err == nil {
		t.Fatalf("Load = nil, want 6 field errors")
	}
	errs := fieldErrors(t, err)

	// One error per bad field, in field order: the fields of DB come at the
	// position of DB, before Debug.
	want := []fieldErrorSummary{
		{Field: "Port", Key: "PORT", Value: "80a"},
		{Field: "Timeout", Key: "TIMEOUT", Value: "30"},
		{Field: "Workers", Key: "WORKERS", Value: "many"}, // Value is the default it tried
		{Field: "DB.URL", Key: "DB_URL"},
		{Field: "DB.Password", Key: "DB_PASSWORD", Secret: true},
		{Field: "Debug", Key: "DEBUG", Value: "yes"},
	}
	if got := summarize(errs); !slices.Equal(got, want) {
		t.Fatalf("Load returned field errors\n got %+v\nwant %+v\nerror:\n%v", got, want, err)
	}

	// Err: ErrRequired for the missing values, the parse error for the others.
	for _, fe := range errs {
		missing := fe.Key == "DB_URL" || fe.Key == "DB_PASSWORD"
		if got := errors.Is(fe, envconfig.ErrRequired); got != missing {
			t.Errorf("errors.Is(<FieldError for %s>, ErrRequired) = %v, want %v (Err: %v)", fe.Key, got, missing, fe.Err)
		}
		if fe.Err == nil {
			t.Errorf("FieldError for %s has Err nil", fe.Key)
		}
	}

	// The joined error works with errors.Is and errors.As, and its message
	// has every key, so one log line shows every mistake.
	if !errors.Is(err, envconfig.ErrRequired) {
		t.Errorf("errors.Is(err, ErrRequired) = false, want true")
	}
	var first *envconfig.FieldError
	if !errors.As(err, &first) || first.Key != "PORT" {
		t.Errorf("errors.As(err, &fieldErr) found %+v, want the FieldError for PORT (the first one)", first)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w.Key) {
			t.Errorf("err.Error() does not mention %s:\n%v", w.Key, err)
		}
	}
}

// dsn is a database URL. Its UnmarshalText returns the error of url.Parse as
// it is, and that error quotes the whole input, password included. Many real
// parsers behave like this.
type dsn struct{ URL *url.URL }

func (d *dsn) UnmarshalText(text []byte) error {
	u, err := url.Parse(string(text))
	if err != nil {
		return err
	}
	d.URL = u
	return nil
}

type secretConfig struct {
	DatabaseURL dsn    `required:"true" secret:"true"`
	APIToken    string `required:"true" secret:"true"`
	Upstream    dsn    // not secret
}

func TestErrorsSecretsNeverInMessage(t *testing.T) {
	const badSecret = "postgres://app:hunter2@db:5432x/orders" // invalid port
	const badPublic = "https://payments.internal:44x3/"        // invalid port
	e := newEnv(map[string]string{"DATABASE_URL": badSecret, "UPSTREAM": badPublic})
	var cfg secretConfig
	err := load(t, &cfg, e)
	errs := fieldErrors(t, err)
	want := []fieldErrorSummary{
		{Field: "DatabaseURL", Key: "DATABASE_URL", Value: badSecret, Secret: true}, // Value keeps the raw text
		{Field: "APIToken", Key: "API_TOKEN", Secret: true},
		{Field: "Upstream", Key: "UPSTREAM", Value: badPublic},
	}
	if got := summarize(errs); !slices.Equal(got, want) {
		t.Fatalf("Load returned field errors\n got %+v\nwant %+v\nerror:\n%v", got, want, err)
	}

	msg := err.Error()
	if strings.Contains(msg, "hunter2") {
		t.Errorf("err.Error() leaks the secret value:\n%s", msg)
	}
	for _, s := range []string{"DATABASE_URL", "API_TOKEN", "UPSTREAM", badPublic} {
		if !strings.Contains(msg, s) {
			t.Errorf("err.Error() does not contain %q:\n%s", s, msg)
		}
	}

	// The cause is still there for code: Err is the error of UnmarshalText.
	var urlErr *url.Error
	if !errors.As(errs[0], &urlErr) {
		t.Errorf("errors.As(<FieldError for DATABASE_URL>, &urlErr) = false, want true (Err: %v)", errs[0].Err)
	}
}

func TestErrorsValidConfigIsNil(t *testing.T) {
	e := newEnv(map[string]string{
		"WORKERS":     "8", // the default of Workers does not parse, so it must be set
		"DB_URL":      "postgres://db.internal/orders",
		"DB_PASSWORD": "hunter2",
		"BROKERS":     "kafka-0:9092",
	})
	var cfg serviceConfig
	if err := load(t, &cfg, e); err != nil {
		t.Fatalf("Load = %v (%T), want nil", err, err)
	}
	if cfg.Port != 8080 || cfg.Timeout != 5*time.Second || cfg.Workers != 8 || cfg.DB.Password != "hunter2" {
		t.Errorf("after Load: %+v, want Port 8080, Timeout 5s, Workers 8, DB.Password hunter2", cfg)
	}
}
