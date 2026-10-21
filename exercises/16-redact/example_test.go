package redact_test

import (
	"encoding/json"
	"fmt"

	redact "github.com/yornik/go-practice/exercises/16-redact"
)

func ExampleSecret() {
	type config struct {
		Host     string        `json:"host"`
		Password redact.Secret `json:"password"`
	}
	cfg := config{Host: "db-1.peckish.example", Password: "hunter2"}

	fmt.Printf("%+v\n", cfg)
	out, _ := json.Marshal(cfg)
	fmt.Println(string(out))
	// Output:
	// {Host:db-1.peckish.example Password:[REDACTED]}
	// {"host":"db-1.peckish.example","password":"[REDACTED]"}
}

func ExampleRedactURL() {
	fmt.Println(redact.RedactURL("postgres://orders_app:hunter2@db-1.peckish.example:5432/orders"))
	fmt.Println(redact.RedactURL("https://hooks.peckish.example/deploy?team=sre&token=abc123"))
	fmt.Println(redact.RedactURL("postgres://orders_app:hun ter@db-1.peckish.example/orders"))
	// Output:
	// postgres://orders_app:xxxxx@db-1.peckish.example:5432/orders
	// https://hooks.peckish.example/deploy?team=sre&token=xxxxx
	// [invalid URL]
}
