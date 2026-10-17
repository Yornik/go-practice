package accesslog_test

import (
	"testing"

	accesslog "github.com/yornik/go-practice/exercises/02-accesslog"
)

type normalizeCase struct {
	name string
	path string
	want string
}

func runNormalizeCases(t *testing.T, cases []normalizeCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := accesslog.NormalizePath(tc.path); got != tc.want {
				t.Errorf("NormalizePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestNormalizePathStatic(t *testing.T) {
	runNormalizeCases(t, []normalizeCase{
		{"root", "/", "/"},
		{"one segment", "/healthz", "/healthz"},
		{"several segments", "/api/v1/users", "/api/v1/users"},
		{"dots and dashes are kept", "/static/app-3.2.1.min.js", "/static/app-3.2.1.min.js"},
		{"letter case is kept", "/Admin/Users", "/Admin/Users"},
	})
}

func TestNormalizePathQueryAndSlashes(t *testing.T) {
	runNormalizeCases(t, []normalizeCase{
		{"query string is dropped", "/search?q=timeout&page=2", "/search"},
		// Everything from the first ? on is the query, also a second ?.
		{"query with a second question mark", "/search?q=why?", "/search"},
		{"query string on the root", "/?utm_source=mail", "/"},
		{"trailing slash is removed", "/api/v1/users/", "/api/v1/users"},
		{"trailing slash before the query", "/api/v1/users/?page=2", "/api/v1/users"},
		{"repeated slashes collapse", "//api///v1//users", "/api/v1/users"},
		{"only slashes", "///", "/"},
		{"empty path", "", "/"},
		{"missing leading slash is added", "users/42", "/users/:id"},
	})
}

func TestNormalizePathIDs(t *testing.T) {
	runNormalizeCases(t, []normalizeCase{
		{"numeric ID", "/users/123", "/users/:id"},
		{"two numeric IDs", "/users/123/orders/456", "/users/:id/orders/:id"},
		{"ID as first segment", "/42", "/:id"},
		{"single digit zero", "/items/0", "/items/:id"},
		{"leading zeros", "/items/007", "/items/:id"},
		// Longer than any int64: "only digits" is about the characters, not
		// about whether the number fits in an int.
		{"25-digit ID", "/orders/1234567890123456789012345", "/orders/:id"},
		{"ID before a trailing slash and query", "/users/42/?tab=keys", "/users/:id"},
		// Not only digits, so kept.
		{"version segment is kept", "/api/v1/users/7", "/api/v1/users/:id"},
		{"letters and digits are kept", "/users/abc123", "/users/abc123"},
		{"digits then letters are kept", "/users/12a", "/users/12a"},
		{"minus sign is kept", "/items/-1", "/items/-1"},
		{"decimal is kept", "/rates/1.5", "/rates/1.5"},
	})
}

func TestNormalizePathUUIDs(t *testing.T) {
	runNormalizeCases(t, []normalizeCase{
		{"lowercase UUID", "/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f", "/orders/:uuid"},
		{"uppercase UUID", "/orders/9F1C2A7E-0B3D-4C1A-9E2F-5D6C7B8A9E0F", "/orders/:uuid"},
		{"mixed case UUID", "/orders/9f1C2a7E-0b3D-4c1A-9e2F-5d6C7b8A9e0F", "/orders/:uuid"},
		{"all-digit UUID is still a UUID", "/orders/12345678-1234-1234-1234-123456789012", "/orders/:uuid"},
		{"UUID and numeric ID", "/users/42/sessions/00000000-0000-0000-0000-000000000000", "/users/:id/sessions/:uuid"},
		{"UUID with query", "/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f?x=1", "/orders/:uuid"},
		// Near misses are not UUIDs and stay as they are.
		{"no hyphens", "/orders/9f1c2a7e0b3d4c1a9e2f5d6c7b8a9e0f", "/orders/9f1c2a7e0b3d4c1a9e2f5d6c7b8a9e0f"},
		{"one character short", "/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0", "/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0"},
		{"one character long", "/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f0", "/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f0"},
		{"g is not a hex digit", "/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0g", "/orders/9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0g"},
		// 36 characters with 4 hyphens, but in the wrong places.
		{"hyphens in the wrong places", "/orders/9f1c2a7e0-b3d-4c1a-9e2f-5d6c7b8a9e0f", "/orders/9f1c2a7e0-b3d-4c1a-9e2f-5d6c7b8a9e0f"},
		{"braces around a UUID", "/orders/{9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f}", "/orders/{9f1c2a7e-0b3d-4c1a-9e2f-5d6c7b8a9e0f}"},
	})
}
