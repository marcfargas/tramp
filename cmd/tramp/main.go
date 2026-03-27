// cmd/tramp/main.go
package main

import (
	"context"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		log.Fatal("Usage: tramp serve")
	}

	ctx := context.Background()
	_ = ctx
	log.Println("tramp MCP server starting...")
}
