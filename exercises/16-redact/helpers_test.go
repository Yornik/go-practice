package redact_test

import (
	"strings"
	"testing"

	redact "github.com/yornik/go-practice/exercises/16-redact"
)

// The secret values in these tests. Each is unique, so a leak shows which
// value got out.
const (
	password = "hunter2"
	token1   = "tok-8f3a91"
	token2   = "tok-c27d40"
)

// dbConfig is the shape of a config struct that holds a password in an
// exported field.
type dbConfig struct {
	Host     string
	User     string
	Password redact.Secret
}

func newDBConfig() dbConfig {
	return dbConfig{Host: "db-1.peckish.example", User: "orders_app", Password: password}
}

// appConfig nests dbConfig and holds secrets in a slice, a map and a
// pointer field.
type appConfig struct {
	Name      string
	DB        dbConfig
	APITokens []redact.Secret
	Webhooks  map[string]redact.Secret
	Backup    *redact.Secret
}

func newAppConfig() appConfig {
	backup := redact.Secret("backup-pw-77")
	return appConfig{
		Name:      "checkout",
		DB:        newDBConfig(),
		APITokens: []redact.Secret{token1, token2},
		Webhooks:  map[string]redact.Secret{"deploy-bot": "whk-5521"},
		Backup:    &backup,
	}
}

// allSecrets are the secret values that newAppConfig holds.
var allSecrets = []string{password, token1, token2, "whk-5521", "backup-pw-77"}

// wantHidden fails the test if out does not contain "[REDACTED]" or
// contains one of the secret values.
func wantHidden(t *testing.T, what, out string, secrets ...string) {
	t.Helper()
	if !strings.Contains(out, "[REDACTED]") {
		t.Errorf("%s = %q, want it to contain %q", what, out, "[REDACTED]")
	}
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s = %q, it contains the secret value %q", what, out, s)
		}
	}
}
