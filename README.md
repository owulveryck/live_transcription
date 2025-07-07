# Live Audio Transcription

Real-time audio transcription with Go backend and web frontend. Captures microphone audio, streams via WebSocket, and provides live transcription using Google Cloud Speech-to-Text and Vertex AI.

## Quick Start

### Prerequisites

- Go 1.24+ installed
- Google Cloud account with an active project
- Google Cloud CLI (`gcloud`) installed

### 1. Google Cloud Setup

**Install and configure gcloud CLI:**
```bash
# Install gcloud CLI (if not already installed)
# Visit: https://cloud.google.com/sdk/docs/install

# Initialize gcloud and authenticate
gcloud init

# Set up Application Default Credentials
gcloud auth application-default login
```

**Create/configure your GCP project:**
```bash
# Set your project ID (replace with your actual project)
export GCP_PROJECT_ID=your-project-id
export GCP_LOCATION=us-central1

# Set the project as default
gcloud config set project $GCP_PROJECT_ID

# Enable required APIs
gcloud services enable speech.googleapis.com
gcloud services enable aiplatform.googleapis.com
```

**Set environment variables:**
```bash
# Add these to your shell profile (.bashrc, .zshrc, etc.)
export GCP_PROJECT_ID=your-project-id
export GCP_LOCATION=us-central1

# Optional: Set custom port (default: 8080)
export PORT=8080
```

### 2. Run the Application

```bash
# Install Go dependencies
go mod tidy

# Run the server
go run main.go
```

### 3. Access the Web Interface

Open your browser and navigate to: http://localhost:8080

## HTTPS Support (Optional)

For secure connections, the server automatically detects SSL certificate files and enables HTTPS:

Note: secure connection is mandatory to use anything else that localhost

### Generate Self-Signed Certificate

```bash
# Generate a private key
openssl genrsa -out server.key 2048

# Generate a self-signed certificate (valid for 365 days)
openssl req -new -x509 -key server.key -out server.crt -days 365

# You'll be prompted for certificate details:
# - Country Name: US
# - State: Your State  
# - City: Your City
# - Organization: Your Organization
# - Organizational Unit: IT Department
# - Common Name: localhost (IMPORTANT: use 'localhost' for local development)
# - Email: your-email@domain.com
```

**Note:** When prompted for "Common Name", enter `localhost` to avoid browser certificate warnings during local development.

### HTTPS Access

Once certificate files (`server.crt` and `server.key`) are present in the project directory:
- Server automatically starts in HTTPS mode
- Access via: https://localhost:8080
- WebSocket connections use: wss://localhost:8080/ws

### Certificate Security

⚠️ **Self-signed certificates will show browser warnings**. For production use, obtain certificates from a trusted Certificate Authority (CA) like Let's Encrypt.

## Features

- Real-time speech transcription with interim/final results
- Multi-language support with auto-detection
- AI-powered summarization (Gemini 2.5 Flash)
- Audio visualization and session statistics
- Copy transcripts and summaries to clipboard

## Configuration

- **Languages**: Configure BCP-47 codes (default: en-US,fr-FR,es-ES)
- **Logging**: Set `LOG_LEVEL` (DEBUG/INFO/WARN/ERROR) and `LOG_FORMAT` (JSON/TEXT)
- **Port**: Set `PORT` environment variable (default: 8080)
- **Audio**: 16kHz LINEAR16 mono format

## API Endpoints

- `GET /` - Web interface
- `GET /api/default-prompt` - Summary prompt configuration
- `WebSocket /ws` - Audio streaming

## Build

```bash
go build -o live_transcription main.go
./live_transcription
```
