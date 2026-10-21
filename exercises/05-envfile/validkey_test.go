package envfile_test

import (
	"testing"

	envfile "github.com/yornik/go-practice/exercises/05-envfile"
)

func TestValidKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"PORT", true},
		{"_x", true},
		{"_", true},
		{"DB_HOST2", true},
		{"db_host", true},
		{"a", true},
		{"Z9", true},
		{"__", true},
		{"export", true},
		{"LOG_LEVEL", true},

		{"", false},
		{"2FA", false},
		{"9", false},
		{"db-host", false},
		{"DB HOST", false},
		{" PORT", false},
		{"PORT ", false},
		{"A.B", false},
		{"A=B", false},
		{"A$", false},
		{"ключ", false},
		{"é", false},
		{"caf\u00e9", false},
		{"PORT\t", false},
	}
	for _, tc := range tests {
		if got := envfile.ValidKey(tc.key); got != tc.want {
			t.Errorf("ValidKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}
