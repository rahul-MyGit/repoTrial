package main

import (
	"log"
	"net/http"
	"os"

	"example.com/repotrial/checkout-idempotency/internal/checkout"
)

func main() {
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	mux := http.NewServeMux()
	checkout.NewHandler(checkout.NewService(checkout.NewMemoryRepository())).Register(mux)
	log.Printf("checkout service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
