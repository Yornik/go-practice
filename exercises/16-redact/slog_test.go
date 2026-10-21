package redact_test

import (
	"bytes"
	"log/slog"
	"testing"

	redact "github.com/yornik/go-practice/exercises/16-redact"
)

// dropTime removes the time from every record, so that the output of a
// handler is the same on every run.
func dropTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}

// logCase logs 1 record with log and compares the handler's output with want.
type logCase struct {
	name string
	log  func(l *slog.Logger)
	want string
}

func runLogCases(t *testing.T, newHandler func(*bytes.Buffer) slog.Handler, cases []logCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			tc.log(slog.New(newHandler(&buf)))
			got := buf.String()
			if got != tc.want {
				t.Errorf("%s\n got  %q\n want %q", tc.name, got, tc.want)
			}
			wantHidden(t, tc.name, got, password)
		})
	}
}

// logCases are the same 5 records for both handlers; want differs per handler.
func logCases(want [5]string) []logCase {
	s := redact.Secret(password)
	return []logCase{
		{`Info("db connect", "host", ..., "password", Secret)`, func(l *slog.Logger) {
			l.Info("db connect", "host", "db-1.peckish.example", "password", s)
		}, want[0]},
		{`Info("db connect", "password", *Secret)`, func(l *slog.Logger) {
			l.Info("db connect", "password", &s)
		}, want[1]},
		{`With("password", Secret).Info("retry")`, func(l *slog.Logger) {
			l.With("password", s).Info("retry")
		}, want[2]},
		{`Info("db connect", slog.Group("db", "password", Secret))`, func(l *slog.Logger) {
			l.Info("db connect", slog.Group("db", "password", s))
		}, want[3]},
		{`Info("config loaded", "db", dbConfig)`, func(l *slog.Logger) {
			l.Info("config loaded", "db", newDBConfig())
		}, want[4]},
	}
}

func TestSecretSlogText(t *testing.T) {
	newHandler := func(buf *bytes.Buffer) slog.Handler {
		return slog.NewTextHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime})
	}
	runLogCases(t, newHandler, logCases([5]string{
		"level=INFO msg=\"db connect\" host=db-1.peckish.example password=[REDACTED]\n",
		"level=INFO msg=\"db connect\" password=[REDACTED]\n",
		"level=INFO msg=retry password=[REDACTED]\n",
		"level=INFO msg=\"db connect\" db.password=[REDACTED]\n",
		"level=INFO msg=\"config loaded\" db=\"{Host:db-1.peckish.example User:orders_app Password:[REDACTED]}\"\n",
	}))
}

func TestSecretSlogJSON(t *testing.T) {
	newHandler := func(buf *bytes.Buffer) slog.Handler {
		return slog.NewJSONHandler(buf, &slog.HandlerOptions{ReplaceAttr: dropTime})
	}
	runLogCases(t, newHandler, logCases([5]string{
		`{"level":"INFO","msg":"db connect","host":"db-1.peckish.example","password":"[REDACTED]"}` + "\n",
		`{"level":"INFO","msg":"db connect","password":"[REDACTED]"}` + "\n",
		`{"level":"INFO","msg":"retry","password":"[REDACTED]"}` + "\n",
		`{"level":"INFO","msg":"db connect","db":{"password":"[REDACTED]"}}` + "\n",
		`{"level":"INFO","msg":"config loaded","db":{"Host":"db-1.peckish.example","User":"orders_app","Password":"[REDACTED]"}}` + "\n",
	}))
}

func TestSecretSlogResolve(t *testing.T) {
	s := redact.Secret(password)

	t.Run("Resolve", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			arg  any
		}{{"Secret", s}, {"*Secret", &s}} {
			v := slog.AnyValue(tc.arg).Resolve()
			if v.Kind() != slog.KindString || v.String() != "[REDACTED]" {
				t.Errorf("slog.AnyValue(%s).Resolve() = kind %v, value %q; want kind %v, value %q",
					tc.name, v.Kind(), v.String(), slog.KindString, "[REDACTED]")
			}
		}
	})

	// A ReplaceAttr function sees the value after slog resolved it.
	t.Run("ReplaceAttr", func(t *testing.T) {
		var seen []slog.Value
		record := func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "password" {
				seen = append(seen, a.Value)
			}
			return a
		}
		var buf bytes.Buffer
		l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{ReplaceAttr: record}))
		l.Info("db connect", "password", s)
		if len(seen) != 1 {
			t.Fatalf(`Info("db connect", "password", Secret): ReplaceAttr saw %d password attributes, want 1`, len(seen))
		}
		if v := seen[0]; v.Kind() != slog.KindString || v.String() != "[REDACTED]" {
			t.Errorf(`Info("db connect", "password", Secret): ReplaceAttr saw kind %v, value %q; want kind %v, value %q`,
				v.Kind(), v.String(), slog.KindString, "[REDACTED]")
		}
	})
}
