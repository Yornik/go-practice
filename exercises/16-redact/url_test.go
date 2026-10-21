package redact_test

import (
	"strings"
	"testing"

	redact "github.com/yornik/go-practice/exercises/16-redact"
)

func TestRedactURLHidesSecrets(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			"postgres password; the query keeps its order",
			"postgres://orders_app:hunter2@db-1.peckish.example:5432/orders?sslmode=require&connect_timeout=5",
			"postgres://orders_app:xxxxx@db-1.peckish.example:5432/orders?sslmode=require&connect_timeout=5",
		},
		{
			"escaped password",
			"postgres://orders_app:p%40ss@db-1.peckish.example/orders",
			"postgres://orders_app:xxxxx@db-1.peckish.example/orders",
		},
		{
			"password without a user name",
			"redis://:hunter2@cache-1.peckish.example:6379/0",
			"redis://:xxxxx@cache-1.peckish.example:6379/0",
		},
		{
			"empty password",
			"postgres://orders_app:@db-1.peckish.example/orders",
			"postgres://orders_app:xxxxx@db-1.peckish.example/orders",
		},
		{
			"Token parameter; keys sorted by byte value",
			"https://hooks.peckish.example/deploy?team=sre&Token=abc123",
			"https://hooks.peckish.example/deploy?Token=xxxxx&team=sre",
		},
		{
			"2 values for 1 key",
			"https://cdn.peckish.example/photos/412.jpg?sig=a1b2&expires=1764000000&sig=c3d4",
			"https://cdn.peckish.example/photos/412.jpg?expires=1764000000&sig=xxxxx&sig=xxxxx",
		},
		{
			"every secret name, in any letter case",
			"https://api.peckish.example/v1?user=ines&password=a&SECRET=b&Key=c&sig=d&TOKEN=e",
			"https://api.peckish.example/v1?Key=xxxxx&SECRET=xxxxx&TOKEN=xxxxx&password=xxxxx&sig=xxxxx&user=ines",
		},
		{
			"empty value",
			"https://hooks.peckish.example/deploy?token=",
			"https://hooks.peckish.example/deploy?token=xxxxx",
		},
		{
			"Encode rewrites the other parameters too",
			"https://search.peckish.example/find?token=abc&q=great%20tit",
			"https://search.peckish.example/find?q=great+tit&token=xxxxx",
		},
		{
			"password and token",
			"https://deploy:s3cret@ci.peckish.example/hook?token=abc&b=1",
			"https://deploy:xxxxx@ci.peckish.example/hook?b=1&token=xxxxx",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redact.RedactURL(tc.raw); got != tc.want {
				t.Errorf("RedactURL(%q)\n got  %q\n want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestRedactURLUnchanged(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"user without password", "postgres://orders_app@db-1.peckish.example:5432/orders?sslmode=require"},
		{"upper-case scheme and unsorted query", "HTTPS://Status.Peckish.example/api/v2/summary.json?q=a%20b&page=2"},
		{"only part of a secret name", "https://shop.peckish.example/search?monkey=1&keys=2&tokens=3"},
		{"escaped slash in the path", "https://photos.peckish.example/feeder%2F0412/latest.jpg"},
		{"relative URL", "/healthz?verbose=1"},
		{"empty string", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := redact.RedactURL(tc.raw); got != tc.raw {
				t.Errorf("RedactURL(%q)\n got  %q\n want the input unchanged", tc.raw, got)
			}
		})
	}
}

func TestRedactURLInvalid(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		secret string
	}{
		{"space in the password", "postgres://orders_app:hun ter@db-1.peckish.example/orders", "hun ter"},
		{"port that is not a number", "postgres://orders_app:hunter2@db-1.peckish.example:port/orders", "hunter2"},
		{"bad escape in the query", "https://hooks.peckish.example/deploy?token=abc123%zz", "abc123"},
		{"semicolon in the query", "https://hooks.peckish.example/deploy?token=abc123;team=sre", "abc123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redact.RedactURL(tc.raw)
			if got != "[invalid URL]" {
				t.Errorf("RedactURL(%q) = %q, want %q", tc.raw, got, "[invalid URL]")
			}
			if strings.Contains(got, tc.secret) {
				t.Errorf("RedactURL(%q) = %q, it contains the secret %q", tc.raw, got, tc.secret)
			}
		})
	}
}
