package envconfig_test

import (
	"slices"
	"testing"
	"time"
)

type tlsFiles struct {
	CertFile string
	KeyFile  string
}

type database struct {
	Host     string `env:"HOSTNAME" required:"true"`
	Port     int    `default:"5432"`
	Password string `secret:"true"`
}

// nestedConfig shows every way to name the fields of a nested struct. The
// comments give the variable names.
type nestedConfig struct {
	DB      database // DB_HOSTNAME, DB_PORT, DB_PASSWORD
	Replica database `env:"DB_RO"` // DB_RO_HOSTNAME, DB_RO_PORT, DB_RO_PASSWORD

	HTTP struct {
		Port        int           `default:"8080"`
		ReadTimeout time.Duration `default:"5s"`
	} `prefix:""` // PORT, READ_TIMEOUT: an empty prefix means no prefix

	Cache struct {
		Addr string
		TTL  time.Duration
	} `prefix:"REDIS_"` // REDIS_ADDR, REDIS_TTL

	Upstream struct {
		URL  string
		TLS  tlsFiles // UPSTREAM_TLS_CERT_FILE, UPSTREAM_TLS_KEY_FILE
		MTLS tlsFiles `prefix:"CLIENT_"` // UPSTREAM_CLIENT_CERT_FILE, UPSTREAM_CLIENT_KEY_FILE
	}

	Debug struct{ Addr string } `env:"-"` // skipped with all its fields
}

func TestNestedKeys(t *testing.T) {
	e := newEnv(map[string]string{
		"DB_HOSTNAME":               "db-primary.internal",
		"DB_PASSWORD":               "hunter2",
		"DB_RO_HOSTNAME":            "db-replica.internal",
		"DB_RO_PORT":                "6432",
		"PORT":                      "9090",
		"REDIS_ADDR":                "redis:6379",
		"REDIS_TTL":                 "30s",
		"UPSTREAM_URL":              "https://payments.internal",
		"UPSTREAM_TLS_CERT_FILE":    "/etc/tls/tls.crt",
		"UPSTREAM_TLS_KEY_FILE":     "/etc/tls/tls.key",
		"UPSTREAM_CLIENT_CERT_FILE": "/etc/client/tls.crt",
		"UPSTREAM_CLIENT_KEY_FILE":  "/etc/client/tls.key",

		// Traps. Kubernetes sets HOSTNAME to the pod name in every pod. The
		// others are what a loader reads if it forgets or misplaces a prefix.
		"HOSTNAME": "checkout-7d9f8b6c4-x2x4q", "PASSWORD": "wrong",
		"HTTP_PORT": "1111", "ADDR": "wrong", "TTL": "1h", "CACHE_ADDR": "wrong",
		"URL": "wrong", "CERT_FILE": "wrong", "TLS_CERT_FILE": "wrong",
		"UPSTREAM_MTLS_CERT_FILE": "wrong", "CLIENT_CERT_FILE": "wrong",
		"DEBUG_ADDR": ":6060", "ADDR_DEBUG": "wrong",
	})

	var want nestedConfig
	want.DB = database{Host: "db-primary.internal", Port: 5432, Password: "hunter2"}
	want.Replica = database{Host: "db-replica.internal", Port: 6432}
	want.HTTP.Port = 9090
	want.HTTP.ReadTimeout = 5 * time.Second
	want.Cache.Addr = "redis:6379"
	want.Cache.TTL = 30 * time.Second
	want.Upstream.URL = "https://payments.internal"
	want.Upstream.TLS = tlsFiles{CertFile: "/etc/tls/tls.crt", KeyFile: "/etc/tls/tls.key"}
	want.Upstream.MTLS = tlsFiles{CertFile: "/etc/client/tls.crt", KeyFile: "/etc/client/tls.key"}

	var got nestedConfig
	mustLoad(t, &got, e)
	if got != want {
		t.Errorf("after Load:\n got %+v\nwant %+v\nkeys looked up: %q", got, want, e.asked)
	}
}

func TestNestedErrorPaths(t *testing.T) {
	// Both required Host fields are missing and one port does not parse.
	// Field is the Go path, Key the full variable name.
	var cfg nestedConfig
	e := newEnv(map[string]string{"UPSTREAM_TLS_CERT_FILE": "/etc/tls/tls.crt", "DB_RO_PORT": "6432x"})
	err := load(t, &cfg, e)
	got := summarize(fieldErrors(t, err))
	want := []fieldErrorSummary{
		{Field: "DB.Host", Key: "DB_HOSTNAME"},
		{Field: "Replica.Host", Key: "DB_RO_HOSTNAME"},
		{Field: "Replica.Port", Key: "DB_RO_PORT", Value: "6432x"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Load returned field errors\n got %+v\nwant %+v\nerror: %v\nkeys looked up: %q", got, want, err, e.asked)
	}
}
