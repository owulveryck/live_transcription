# Live Audio Transcription

Real-time audio transcription with Go backend and web frontend. Captures microphone audio, streams via WebSocket, and provides live transcription using Google Cloud Speech-to-Text and Vertex AI.

## Quick Start

1. **Set up Google Cloud:**
   ```bash
   export GCP_PROJECT_ID=your-project-id
   export GCP_LOCATION=us-central1
   ```
   Enable Speech-to-Text API and Vertex AI API in your GCP project.

2. **Run the server:**
   ```bash
   go run main.go
   ```

3. **Open browser:**
   Go to http://localhost:8080 and start transcribing.

## Features

- Real-time speech transcription with interim/final results
- Multi-language support with auto-detection
- AI-powered summarization (Gemini 2.5 Flash)
- Audio visualization and session statistics
- Copy transcripts and summaries to clipboard

## Configuration

- **Languages**: Configure BCP-47 codes (default: en-US,fr-FR,es-ES)
- **Logging**: Set `LOG_LEVEL` (DEBUG/INFO/WARN/ERROR) and `LOG_FORMAT` (JSON/TEXT)
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