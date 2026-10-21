package deployorder_test

import (
	"fmt"
	"log"
	"strings"

	deployorder "github.com/yornik/go-practice/exercises/35-deployorder"
)

func Example() {
	g, err := deployorder.New(map[string][]string{
		"web":      {"api"},
		"api":      {"postgres", "redis"},
		"worker":   {"postgres"},
		"postgres": nil,
		"redis":    nil,
	})
	if err != nil {
		log.Fatal(err)
	}

	waves, err := g.Waves()
	if err != nil {
		log.Fatal(err)
	}
	for i, w := range waves {
		fmt.Printf("wave %d: %s\n", i, strings.Join(w, ", "))
	}

	blast, _ := g.Dependents("postgres")
	fmt.Println("if postgres is down:", strings.Join(blast, ", "))

	chain, _ := g.Why("web", "redis")
	fmt.Println("why web needs redis:", strings.Join(chain, " -> "))

	_, err = deployorder.New(map[string][]string{"api": {"postgress"}, "postgres": nil})
	fmt.Println(err)

	g, _ = deployorder.New(map[string][]string{"auth": {"config"}, "config": {"auth"}, "web": {"auth"}})
	_, err = g.Waves()
	fmt.Println(err)

	// Output:
	// wave 0: postgres, redis
	// wave 1: api, worker
	// wave 2: web
	// if postgres is down: api, web, worker
	// why web needs redis: web -> api -> redis
	// service "api" depends on unknown service "postgress"
	// dependency cycle: auth -> config -> auth
}
