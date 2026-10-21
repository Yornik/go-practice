package envconfig_test

import (
	"testing"

	envconfig "github.com/yornik/go-practice/exercises/31-envconfig"
)

func TestUpperSnake(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		// One word, several words.
		{"Port", "PORT"},
		{"ReadTimeout", "READ_TIMEOUT"},
		{"MaxIdleConnsPerHost", "MAX_IDLE_CONNS_PER_HOST"},

		// Acronyms stay together. A new word starts at the last capital of
		// an acronym when a lower-case letter follows it.
		{"HTTPPort", "HTTP_PORT"},
		{"APIKey", "API_KEY"},
		{"TLSCertFile", "TLS_CERT_FILE"},
		{"GRPCMaxRecvMsgSize", "GRPC_MAX_RECV_MSG_SIZE"},
		{"UserID", "USER_ID"},
		{"DatabaseURL", "DATABASE_URL"},
		{"ID", "ID"},
		{"DB", "DB"},
		{"X", "X"},

		// Digits belong to the word before them. A capital after a digit
		// starts a new word.
		{"MaxConns2", "MAX_CONNS2"},
		{"S3Bucket", "S3_BUCKET"},
		{"HTTP2Enabled", "HTTP2_ENABLED"},
		{"Port80Redirect", "PORT80_REDIRECT"},

		// A lower-case start is fine (UpperSnake does not care whether the
		// name is exported), and so is the empty string.
		{"lowerCase", "LOWER_CASE"},
		{"", ""},
	}
	defer catchPanic(t)
	for _, tc := range tests {
		if got := envconfig.UpperSnake(tc.name); got != tc.want {
			t.Errorf("UpperSnake(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
