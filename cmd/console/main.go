// Command pluris-console runs the Pluris management console.
//
// Increment 1: ships only the navigation shell. No backend yet.
package main

import (
	"log"
	"os"

	"github.com/pluris/pluris/console/server"
)

func main() {
	addr := os.Getenv("PLURIS_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	e := server.New()
	log.Printf("pluris-console listening on %s", addr)
	if err := e.Start(addr); err != nil {
		log.Fatal(err)
	}
}
