package redact_test

import (
	"encoding/json"
	"testing"

	redact "github.com/yornik/go-practice/exercises/16-redact"
)

// dbConfigJSON is dbConfig with the JSON names a config file uses.
type dbConfigJSON struct {
	Host     string        `json:"host"`
	Password redact.Secret `json:"password"`
}

// backupConfigJSON holds the secret behind a pointer.
type backupConfigJSON struct {
	Host     string         `json:"host"`
	Password *redact.Secret `json:"password"`
}

func TestSecretJSONMarshal(t *testing.T) {
	s := redact.Secret(password)
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"Secret", s, `"[REDACTED]"`},
		{"*Secret", &s, `"[REDACTED]"`},
		{`Secret("")`, redact.Secret(""), `"[REDACTED]"`},
		{"struct field", dbConfigJSON{Host: "db-1.peckish.example", Password: password},
			`{"host":"db-1.peckish.example","password":"[REDACTED]"}`},
		{"pointer to struct", &dbConfigJSON{Host: "db-1.peckish.example", Password: password},
			`{"host":"db-1.peckish.example","password":"[REDACTED]"}`},
		{"*Secret struct field", backupConfigJSON{Host: "db-2.peckish.example", Password: &s},
			`{"host":"db-2.peckish.example","password":"[REDACTED]"}`},
		{"slice", []redact.Secret{token1, token2}, `["[REDACTED]","[REDACTED]"]`},
		{"map value", map[string]redact.Secret{"deploy-bot": token1}, `{"deploy-bot":"[REDACTED]"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatalf("json.Marshal(%s) error = %v, want nil", tc.name, err)
			}
			if string(got) != tc.want {
				t.Errorf("json.Marshal(%s) = %s, want %s", tc.name, got, tc.want)
			}
		})
	}
}

func TestSecretJSONUnmarshal(t *testing.T) {
	t.Run("Secret", func(t *testing.T) {
		var s redact.Secret
		in := `"hunter2"`
		if err := json.Unmarshal([]byte(in), &s); err != nil {
			t.Fatalf("json.Unmarshal(%s) error = %v, want nil", in, err)
		}
		if got := s.Reveal(); got != password {
			t.Errorf("after json.Unmarshal(%s): Reveal() = %q, want %q", in, got, password)
		}
	})

	t.Run("struct field", func(t *testing.T) {
		var c dbConfigJSON
		in := `{"host":"db-1.peckish.example","password":"hunter2"}`
		if err := json.Unmarshal([]byte(in), &c); err != nil {
			t.Fatalf("json.Unmarshal(%s) error = %v, want nil", in, err)
		}
		if c.Host != "db-1.peckish.example" {
			t.Errorf("after json.Unmarshal(%s): Host = %q, want %q", in, c.Host, "db-1.peckish.example")
		}
		if got := c.Password.Reveal(); got != password {
			t.Errorf("after json.Unmarshal(%s): Password.Reveal() = %q, want %q", in, got, password)
		}
	})

	t.Run("not a string", func(t *testing.T) {
		for _, in := range []string{`42`, `true`, `{"value":"hunter2"}`, `["hunter2"]`} {
			var s redact.Secret
			if err := json.Unmarshal([]byte(in), &s); err == nil {
				t.Errorf("json.Unmarshal(%s) into a Secret: error = nil, want an error", in)
			}
		}
	})
}

func TestSecretReveal(t *testing.T) {
	for _, want := range []string{password, "p@ss word", ""} {
		s := redact.Secret(want)
		if got := s.Reveal(); got != want {
			t.Errorf("Secret(%q).Reveal() = %q, want %q", want, got, want)
		}
		if got := (&s).Reveal(); got != want {
			t.Errorf("(&Secret(%q)).Reveal() = %q, want %q", want, got, want)
		}
	}
}
