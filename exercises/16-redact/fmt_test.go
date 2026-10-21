package redact_test

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"testing"

	redact "github.com/yornik/go-practice/exercises/16-redact"
)

func TestSecretFmtVerbs(t *testing.T) {
	s := redact.Secret(password)
	empty := redact.Secret("")
	cases := []struct {
		format string
		arg    any
		argDoc string
		want   string
	}{
		{"%v", s, "Secret", "[REDACTED]"},
		{"%+v", s, "Secret", "[REDACTED]"},
		{"%s", s, "Secret", "[REDACTED]"},
		{"%#v", s, "Secret", "[REDACTED]"},
		{"%q", s, "Secret", `"[REDACTED]"`},
		{"%x", s, "Secret", "5b52454441435445445d"},
		{"%X", s, "Secret", "5B52454441435445445D"},

		{"%v", &s, "*Secret", "[REDACTED]"},
		{"%+v", &s, "*Secret", "[REDACTED]"},
		{"%s", &s, "*Secret", "[REDACTED]"},
		{"%#v", &s, "*Secret", "[REDACTED]"},
		{"%q", &s, "*Secret", `"[REDACTED]"`},
		{"%x", &s, "*Secret", "5b52454441435445445d"},
		{"%X", &s, "*Secret", "5B52454441435445445D"},

		// The output must not tell whether a secret is set.
		{"%v", empty, `Secret("")`, "[REDACTED]"},
		{"%#v", empty, `Secret("")`, "[REDACTED]"},
		{"%q", empty, `Secret("")`, `"[REDACTED]"`},

		{"password=%s", s, "Secret", "password=[REDACTED]"},
	}
	for _, tc := range cases {
		t.Run(tc.format+" "+tc.argDoc, func(t *testing.T) {
			got := fmt.Sprintf(tc.format, tc.arg)
			if got != tc.want {
				t.Errorf("fmt.Sprintf(%q, %s) = %q, want %q", tc.format, tc.argDoc, got, tc.want)
			}
		})
	}
}

func TestSecretFmtPrint(t *testing.T) {
	s := redact.Secret(password)
	cases := []struct {
		name string
		got  func() string
		want string
	}{
		{"Sprint(Secret)", func() string { return fmt.Sprint(s) }, "[REDACTED]"},
		{"Sprint(*Secret)", func() string { return fmt.Sprint(&s) }, "[REDACTED]"},
		{"Sprintln(Secret)", func() string { return fmt.Sprintln(s) }, "[REDACTED]\n"},
		{"Sprintln(*Secret)", func() string { return fmt.Sprintln(&s) }, "[REDACTED]\n"},
		{`Sprintln("password:", Secret)`, func() string { return fmt.Sprintln("password:", s) }, "password: [REDACTED]\n"},
		{"Fprintln(&buf, Secret)", func() string {
			var buf bytes.Buffer
			fmt.Fprintln(&buf, s)
			return buf.String()
		}, "[REDACTED]\n"},
		{"Fprint(&buf, Secret)", func() string {
			var buf bytes.Buffer
			fmt.Fprint(&buf, s)
			return buf.String()
		}, "[REDACTED]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got(); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestSecretNested(t *testing.T) {
	db := newDBConfig()
	app := newAppConfig()
	tokens := []redact.Secret{token1, token2}
	hooks := map[string]redact.Secret{"deploy-bot": token1, "status-page": token2}
	var logged bytes.Buffer
	logger := log.New(&logged, "", 0)
	logger.Printf("config: %+v", app)

	cases := []struct {
		what    string
		out     string
		secrets []string
	}{
		{`Sprintf("%v", dbConfig)`, fmt.Sprintf("%v", db), []string{password}},
		{`Sprintf("%+v", dbConfig)`, fmt.Sprintf("%+v", db), []string{password}},
		{`Sprintf("%#v", dbConfig)`, fmt.Sprintf("%#v", db), []string{password}},
		{`Sprintf("%+v", &dbConfig)`, fmt.Sprintf("%+v", &db), []string{password}},
		{`Sprintf("%#v", &dbConfig)`, fmt.Sprintf("%#v", &db), []string{password}},
		{`Sprint(dbConfig)`, fmt.Sprint(db), []string{password}},

		{`Sprintf("%v", []Secret)`, fmt.Sprintf("%v", tokens), []string{token1, token2}},
		{`Sprintf("%+v", []Secret)`, fmt.Sprintf("%+v", tokens), []string{token1, token2}},
		{`Sprintf("%#v", []Secret)`, fmt.Sprintf("%#v", tokens), []string{token1, token2}},

		{`Sprintf("%v", map[string]Secret)`, fmt.Sprintf("%v", hooks), []string{token1, token2}},
		{`Sprintf("%+v", map[string]Secret)`, fmt.Sprintf("%+v", hooks), []string{token1, token2}},
		{`Sprintf("%#v", map[string]Secret)`, fmt.Sprintf("%#v", hooks), []string{token1, token2}},

		// appConfig nests dbConfig and has a slice, a map and a *Secret field.
		{`Sprintf("%v", appConfig)`, fmt.Sprintf("%v", app), allSecrets},
		{`Sprintf("%+v", appConfig)`, fmt.Sprintf("%+v", app), allSecrets},
		{`Sprintf("%#v", appConfig)`, fmt.Sprintf("%#v", app), allSecrets},
		{`Sprintf("%+v", &appConfig)`, fmt.Sprintf("%+v", &app), allSecrets},

		{`log.Printf("config: %+v", appConfig)`, logged.String(), allSecrets},
		{
			`fmt.Errorf("connect as %s with %v: %w", ...)`,
			fmt.Errorf("connect as %s with %v: %w", db.User, db.Password, errors.New("auth failed")).Error(),
			[]string{password},
		},
	}
	for _, tc := range cases {
		t.Run(tc.what, func(t *testing.T) {
			wantHidden(t, tc.what, tc.out, tc.secrets...)
		})
	}
}
