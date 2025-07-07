# Makefile for Live Transcription Application

# Binary name
BINARY_NAME=live_transcription

# Certificate files
CERT_DIR=certs
CERT_FILE=$(CERT_DIR)/server.crt
KEY_FILE=$(CERT_DIR)/server.key

# Default certificate values
CERT_SUBJECT="/C=US/ST=CA/L=San Francisco/O=Live Transcription/CN=localhost"
CERT_DAYS=365

.PHONY: all build cert clean help

# Default target
all: cert build

# Build the Go binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) .

# Generate self-signed certificate
cert: $(CERT_FILE)

$(CERT_FILE): | $(CERT_DIR)
	@echo "Generating self-signed certificate..."
	openssl req -x509 -newkey rsa:4096 -keyout $(KEY_FILE) -out $(CERT_FILE) \
		-days $(CERT_DAYS) -nodes -subj $(CERT_SUBJECT) \
		-addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
	@echo "Certificate generated: $(CERT_FILE)"
	@echo "Private key generated: $(KEY_FILE)"

# Create certificate directory
$(CERT_DIR):
	@mkdir -p $(CERT_DIR)

# Clean build artifacts and certificates
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)
	rm -rf $(CERT_DIR)

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy

# Run the application
run: build
	./$(BINARY_NAME)

# Development build with race detection
dev:
	go run -race main.go

# Help target
help:
	@echo "Available targets:"
	@echo "  all       - Generate certificate and build binary (default)"
	@echo "  build     - Build the Go binary"
	@echo "  cert      - Generate self-signed certificate"
	@echo "  clean     - Remove binary and certificates"
	@echo "  deps      - Install Go dependencies"
	@echo "  run       - Build and run the application"
	@echo "  dev       - Run in development mode with race detection"
	@echo "  help      - Show this help message"
