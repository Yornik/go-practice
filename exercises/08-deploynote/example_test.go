package deploynote_test

import (
	"errors"
	"fmt"
	"time"

	deploynote "github.com/yornik/go-practice/exercises/08-deploynote"
)

func ExampleCheck() {
	cet := time.FixedZone("CET", 60*60)
	note := deploynote.Note{
		Service:  "alert-rules",
		Version:  "v1.5.0",
		Ticket:   "small fix",
		Author:   "hush-bot",
		Rollback: "n/a",
		At:       time.Date(2026, time.November, 6, 17, 40, 0, 0, cet), // a Friday
	}

	err := deploynote.Check(note)
	fmt.Println(err)

	var fe *deploynote.FieldError
	if errors.As(err, &fe) {
		fmt.Println("first problem:", fe.Field)
	}
	fmt.Println("frozen:", errors.Is(err, deploynote.ErrFreeze))

	note.Ticket = "INC-45"
	note.Rollback = "helm rollback alert-rules 41"
	fmt.Println("with an incident ticket:", deploynote.Check(note))
	// Output:
	// Ticket: bad format: "small fix"
	// Rollback: missing
	// At: change freeze
	// first problem: Ticket
	// frozen: true
	// with an incident ticket: <nil>
}
