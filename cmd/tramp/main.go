package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	trampmcp "github.com/marcfargas/tramp/internal/mcp"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "Usage: tramp serve")
		os.Exit(1)
	}

	// Determine project directory (cwd)
	projectDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Cannot determine working directory: %v", err)
	}

	// Create service with config loading
	svc := trampmcp.NewService(projectDir)

	// Create MCP server
	server := trampmcp.NewServer(svc)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		svc.Pool.CloseAll()
		cancel()
	}()

	// Run MCP server over stdio
	if err := server.Run(ctx, &gomcp.StdioTransport{}); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
