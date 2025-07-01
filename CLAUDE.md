# Live Audio Transcription

## Project Overview

This is a real-time audio transcription application that uses Google Cloud Speech-to-Text API and Vertex AI for live speech recognition and summarization. The application captures audio from the user's microphone, transcribes it in real-time, and generates summaries using Google's Gemini AI model.

## Architecture

- **Backend**: Go-based WebSocket server (`main.go`)
- **Frontend**: HTML/JavaScript web interface (`live_transcription_ui.html`, `live_audio_recorder.js`)
- **APIs**: Google Cloud Speech-to-Text and Vertex AI (Gemini 2.5 Flash)

## Features

- Real-time audio transcription with interim and final results
- Multi-language support with automatic language detection
- Live audio visualization
- AI-powered summarization of transcribed content
- Copy-to-clipboard functionality for transcripts and summaries
- Session statistics (word count, transcript count, session time)

## Setup

### Prerequisites

1. Google Cloud Project with enabled APIs:
   - Speech-to-Text API
   - Vertex AI API
2. Google Cloud authentication (service account or ADC)

### Environment Variables

```bash
GCP_PROJECT_ID=your-gcp-project-id
GCP_LOCATION=your-gcp-location  # e.g., us-central1
SUMMARY_PROMPT="Your custom summarization prompt"  # Optional
```

### Installation & Running

```bash
# Install dependencies
go mod tidy

# Build the application
go build -o live_transcription

# Run the server
./live_transcription
```

The server will start on `http://localhost:8080`

## Usage

1. Open `http://localhost:8080` in your web browser
2. Select your audio input device
3. Configure language codes (default: en-US,fr-FR,es-ES)
4. Click "Start Live Transcription"
5. Speak into your microphone to see real-time transcription
6. View AI-generated summaries of your speech

## API Endpoints

- `GET /`: Serves the web interface
- `GET /live_audio_recorder.js`: Serves the JavaScript client
- `WebSocket /ws`: Real-time audio streaming and transcription

## Configuration

- **Audio Format**: LINEAR16, 16kHz sample rate, mono
- **Language Detection**: Supports multiple BCP-47 language codes
- **Summarization**: Configurable prompt via `SUMMARY_PROMPT` environment variable

## Development Commands

```bash
# Run the application
go run main.go

# Build for production
go build -o live_transcription main.go
```

## File Structure

- `main.go` - Go backend server with WebSocket handling
- `live_transcription_ui.html` - Web interface
- `live_audio_recorder.js` - JavaScript audio recording and WebSocket client
- `go.mod` - Go module dependencies