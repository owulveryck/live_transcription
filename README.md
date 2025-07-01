# Live Audio Transcription Sample

This is a complete audio transcription system with a Go backend and JavaScript frontend that captures audio from the browser, streams it via WebSocket, and displays real-time transcriptions.

## Features

- Real-time audio capture from microphone or system audio
- WebSocket streaming for low-latency audio transmission  
- Live transcription display with visual feedback
- Audio visualizer with frequency bars
- Session statistics (transcript count, word count, session time)
- Multiple recording modes (microphone, system audio, both)

## Quick Start

1. **Build and run the Go backend:**
   ```bash
   go build -o live_transcription main.go
   ./live_transcription
   ```

2. **Open the frontend:**
   - Navigate to http://localhost:8080 in your browser
   - The UI will load automatically

3. **Start transcription:**
   - Ensure microphone permissions are granted
   - Click "Start Live Transcription"
   - Speak into your microphone
   - See real-time transcriptions appear in the output area

## Architecture

### Backend (main.go)
- Go HTTP server with WebSocket support
- Handles audio format negotiation
- Processes incoming audio chunks
- Mock transcription service (easily replaceable with real transcription APIs)
- Sends JSON responses back to frontend

### Frontend
- **live_transcription_ui.html**: Complete UI with controls and display
- **live_audio_recorder.js**: Audio capture and WebSocket communication
- Features audio device selection, recording modes, and real-time visualization

## Audio Processing Flow

1. Frontend captures audio using MediaRecorder API
2. Audio is streamed as binary data over WebSocket in 250ms chunks
3. Backend receives audio chunks and processes them
4. Mock transcription generates sample text based on chunk size
5. Transcription results are sent back as JSON messages
6. Frontend displays results with timestamps and formatting

## Customization

### Adding Real Transcription
Replace the mock transcription in `main.go` with your preferred service:

```go
func (at *AudioTranscriber) TranscribeAudio(audioData []byte, format AudioFormat) (string, error) {
    // Replace with real transcription service:
    // - Google Speech-to-Text API
    // - Azure Speech Services  
    // - AWS Transcribe Streaming
    // - OpenAI Whisper
    // - Local transcription models
}
```

### Audio Format Support
Currently supports:
- audio/wav (preferred for quality)
- audio/webm;codecs=opus (common browser default)
- audio/ogg;codecs=opus
- audio/opus

### Recording Modes
- **Microphone**: Standard microphone input
- **System Audio**: Capture system audio (Chrome/Edge with screen sharing)
- **Both**: Mix microphone and system audio

## Testing

The system was tested and verified to work with:
- WebSocket connection establishment
- Audio format negotiation (opus codec)
- Real-time audio streaming (4.8KB chunks)
- Bidirectional communication
- Clean connection termination

## Dependencies

- Go: gorilla/websocket for WebSocket handling
- Browser: MediaRecorder API, WebSocket API, getUserMedia/getDisplayMedia

## Development Notes

- CORS is disabled for development (update for production)
- Mock transcription provides varied sample text
- Audio visualizer shows live frequency data
- Session statistics track usage metrics
- Responsive UI works on desktop and mobile browsers

---

# Original Components Documentation

This directory contains extracted audio recording components for live streaming audio to a generative AI inference engine for real-time transcription.

## Components

### 1. `live_audio_recorder.js`
A standalone JavaScript class that handles:
- Audio device enumeration and selection
- Microphone and system audio capture
- Audio stream mixing (for recording both mic and system audio)
- Real-time audio streaming via WebSocket
- Audio visualization
- Recording time tracking

**Key Features:**
- Supports multiple recording modes: microphone only, system audio only, or both
- Uses Opus codec for efficient audio streaming
- Real-time audio visualizer
- Automatic audio device detection
- WebSocket-based live streaming

### 2. `live_transcription_ui.html`
A complete web interface that demonstrates how to use the `LiveAudioRecorder` class:
- Clean, responsive UI for audio recording controls
- Real-time connection status indicator
- Live transcription output display
- Audio visualizer
- Recording time display

### 3. `live_streaming.html` (existing)
The original Gemini 2.0 live streaming demo for reference.

### 4. `main.go` (existing)
Go backend that handles WebSocket connections to Gemini 2.0 API.

## Usage

### Quick Start with Live Transcription UI

1. **Start the backend server:**
   ```bash
   cd sample/
   go run main.go
   ```

2. **Open the live transcription UI:**
   Open `live_transcription_ui.html` in your web browser.

3. **Configure settings:**
   - Select your audio input device
   - Choose recording mode (microphone, system audio, or both)
   - Verify WebSocket URL (default: `ws://localhost:8080/stream`)

4. **Start live transcription:**
   - Click "Start Live Transcription"
   - Allow microphone/screen sharing permissions when prompted
   - Speak into your microphone to see live transcription results

### Integration with Your Own Backend

To integrate with a different backend endpoint:

1. **Include the audio recorder:**
   ```html
   <script src="live_audio_recorder.js"></script>
   ```

2. **Initialize the recorder:**
   ```javascript
   const recorder = new LiveAudioRecorder();
   await recorder.populateAudioDevices();
   ```

3. **Start recording with your WebSocket URL:**
   ```javascript
   await recorder.startRecording('ws://your-backend:port/your-endpoint');
   ```

4. **Listen for transcription events:**
   ```javascript
   document.addEventListener('transcription', function(event) {
       const transcriptionData = event.detail;
       console.log('Received transcription:', transcriptionData);
   });
   ```

### Backend Requirements

Your backend WebSocket endpoint should:

1. **Accept audio format information:**
   ```json
   {
     "format": "audio/webm;codecs=opus",
     "sampleRate": 44100,
     "channels": 1
   }
   ```

2. **Receive audio chunks:**
   Binary audio data chunks sent via WebSocket

3. **Send transcription results:**
   ```json
   {
     "text": "transcribed text",
     "timestamp": 1234567890,
     "final": true
   }
   ```

## Audio Formats

The recorder automatically selects the best supported audio format:
- **Primary:** `audio/webm;codecs=opus` (best for voice, widely supported)
- **Fallback 1:** `audio/ogg;codecs=opus`
- **Fallback 2:** `audio/opus`
- **Last resort:** Browser default format

## Recording Modes

### Microphone Only
- Records from selected microphone device
- Best for voice transcription
- Lowest latency

### System Audio Only
- Records computer's audio output (speakers)
- Requires screen sharing permission
- Useful for transcribing media playback

### Both (Mixed)
- Records both microphone and system audio
- Audio streams are mixed using Web Audio API
- Requires both microphone and screen sharing permissions

## Browser Compatibility

- **Chrome/Edge:** Full support for all features
- **Firefox:** Microphone recording only (system audio limited)
- **Safari:** Basic microphone recording support

## Security Considerations

- HTTPS required for production deployment
- Microphone permission required
- Screen sharing permission required for system audio
- WebSocket connections should use WSS in production

## Troubleshooting

### Common Issues

1. **"Permission denied" errors:**
   - Ensure HTTPS is used in production
   - Check browser permissions for microphone/screen sharing

2. **"System audio not supported" errors:**
   - Use Chrome or Edge browsers
   - Ensure "Share audio" is selected during screen sharing

3. **No audio devices found:**
   - Check microphone hardware connections
   - Verify browser has permission to access audio devices

4. **WebSocket connection fails:**
   - Verify backend server is running
   - Check WebSocket URL format
   - Ensure CORS policies allow WebSocket connections

### Debug Mode

Enable debug logging by opening browser developer tools. The recorder logs detailed information about:
- Audio device enumeration
- Stream creation and mixing
- MediaRecorder format selection
- WebSocket connection status
- Audio chunk transmission

## Examples

See `live_transcription_ui.html` for a complete working example that demonstrates all features of the live audio recorder.