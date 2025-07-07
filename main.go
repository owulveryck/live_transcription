package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func main() {
	// Initialize logging
	initLogger()

	// Set up routes
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/api/default-prompt", serveDefaultPrompt)
	http.HandleFunc("/", serveStaticFiles)

	// Get port from environment variable, default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	// Ensure port has colon prefix
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	// Get certificate file paths from environment variables or use defaults
	certFile := os.Getenv("CERT_FILE")
	if certFile == "" {
		certFile = "certs/server.crt"
	}
	
	keyFile := os.Getenv("KEY_FILE")
	if keyFile == "" {
		keyFile = "certs/server.key"
	}

	// Check if certificate files exist
	_, certErr := os.Stat(certFile)
	_, keyErr := os.Stat(keyFile)

	if certErr == nil && keyErr == nil {
		// Both certificate files exist, start HTTPS server
		logger.Info("Certificate files found, starting HTTPS server",
			"address", fmt.Sprintf("https://localhost%s", port),
			"websocket", fmt.Sprintf("wss://localhost%s/ws", port),
			"certFile", certFile,
			"keyFile", keyFile)

		if err := http.ListenAndServeTLS(port, certFile, keyFile, nil); err != nil {
			logger.Error("HTTPS server failed to start", "error", err)
			os.Exit(1)
		}
	} else {
		// Certificate files not found, start HTTP server
		logger.Info("Starting HTTP server",
			"address", fmt.Sprintf("http://localhost%s", port),
			"websocket", fmt.Sprintf("ws://localhost%s/ws", port),
			"note", fmt.Sprintf("For HTTPS, place certificate files at %s and %s", certFile, keyFile))

		if err := http.ListenAndServe(port, nil); err != nil {
			logger.Error("HTTP server failed to start", "error", err)
			os.Exit(1)
		}
	}
}