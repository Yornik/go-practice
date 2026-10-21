package envconfig_test

import (
	"fmt"
	"log/slog"
	"time"

	envconfig "github.com/yornik/go-practice/exercises/31-envconfig"
)

func ExampleLoad() {
	type Config struct {
		Port        int           `default:"8080"`
		ReadTimeout time.Duration `default:"5s"`
		LogLevel    slog.Level    `default:"info"`
		DB          struct {
			Host     string `required:"true"`
			Password string `required:"true" secret:"true"`
		}
	}

	// In production: envconfig.Load(&cfg, os.LookupEnv).
	env := map[string]string{"PORT": "9090", "LOG_LEVEL": "debug", "DB_HOST": "db.internal", "DB_PASSWORD": "hunter2"}
	lookup := func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}

	var cfg Config
	fmt.Println(envconfig.Load(&cfg, lookup))
	fmt.Println(cfg.Port, cfg.ReadTimeout, cfg.LogLevel, cfg.DB.Host)

	// Two mistakes in one deploy: both reported, one line each, in field order.
	// DB.Host still holds "db.internal" from the first Load, but required
	// checks the environment, not what cfg holds.
	env["READ_TIMEOUT"] = "30"
	delete(env, "DB_HOST")
	fmt.Println(envconfig.Load(&cfg, lookup))

	// Output:
	// <nil>
	// 9090 5s DEBUG db.internal
	// READ_TIMEOUT="30": time: missing unit in duration "30"
	// DB_HOST: required variable is not set
}
