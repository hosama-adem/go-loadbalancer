package main

import (
	"log"
	"net/http"

	loadbalancer "github.com/hosama-adem/go-loadbalancer"
)

func main() {
	lb := loadbalancer.New()

	backends := []string{
		"http://localhost:8081",
		"http://localhost:8082",
		"http://localhost:8083",
	}

	for _, backend := range backends {
		if err := lb.AddBackend(backend); err != nil {
			log.Fatal(err)
		}
	}

	log.Println("Load balancer running on :3030")

	if err := http.ListenAndServe(":3030", lb); err != nil {
		log.Fatal(err)
	}
}
