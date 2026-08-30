package main

import (
	"log"
	"net/http"
	"os"

	"github.com/tokpath/tokstudio/backend/internal/gateway"
)

func main() {
	addr := os.Getenv("TOKENHUB_HTTP_ADDR")
	if addr == "" {
		addr = ":8081"
	}
	log.Printf("bifrost sandbox listening on %s", addr)
	if err := http.ListenAndServe(addr, gateway.SandboxHandler()); err != nil {
		log.Fatal(err)
	}
}
