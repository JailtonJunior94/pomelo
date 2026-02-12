package main

import (
	"os"

	"github.com/jailtonjunior/pomelo/simulator/mcp"
)

func main() {
	baseURL := os.Getenv("WEBHOOK_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	server := mcp.NewServer(baseURL)
	server.Run()
}
