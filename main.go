package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	speech "cloud.google.com/go/speech/apiv1"
	"github.com/gorilla/websocket"
	"google.golang.org/genai"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

type AudioFormat struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

// ConfigMessage represents the initial configuration sent from the client
type ConfigMessage struct {
	Type                     string      `json:"type"`
	AudioFormat              AudioFormat `json:"audioFormat"`
	LanguageCode             string      `json:"languageCode"`
	AlternativeLanguageCodes []string    `json:"alternativeLanguageCodes"`
}

// TranscriptionResponse represents the transcription response sent back to the client
type TranscriptionResponse struct {
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
	Final     bool      `json:"final"`
}

// SummaryResponse represents the summary response sent back to the client
type SummaryResponse struct {
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	Timestamp time.Time `json:"timestamp"`
}

// StatusResponse represents status updates sent to the client
type StatusResponse struct {
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin for development
	},
}

// Global logger
var logger *slog.Logger

// initLogger initializes the structured logger based on configuration
func initLogger() {
	// Get log level from environment variable, default to INFO
	logLevel := os.Getenv("LOG_LEVEL")
	var level slog.Level
	switch strings.ToUpper(logLevel) {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN", "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo // Default to INFO
	}

	// Get log format from environment variable, default to JSON
	logFormat := os.Getenv("LOG_FORMAT")
	var handler slog.Handler
	switch strings.ToUpper(logFormat) {
	case "TEXT":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	case "JSON", "":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	default:
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	logger = slog.New(handler)
	slog.SetDefault(logger)
}

// generateSummary uses Google GenAI to generate content based on the provided transcript, previous summary, and prompt
func generateSummary(ctx context.Context, projectID, location, fullTranscript, previousSummary, prompt string) (string, error) {
	if fullTranscript == "" {
		return "", nil
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  projectID,
		Location: location,
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		return "", fmt.Errorf("error creating GenAI client: %v", err)
	}

	// Build the full prompt with transcript and previous summary
	var fullPrompt string
	if previousSummary != "" {
		fullPrompt = fmt.Sprintf("%s\n\n--- PREVIOUS SUMMARY ---\n%s\n\n--- FULL TRANSCRIPT ---\n%s", prompt, previousSummary, fullTranscript)
	} else {
		fullPrompt = fmt.Sprintf("%s\n\n--- FULL TRANSCRIPT ---\n%s", prompt, fullTranscript)
	}

	parts := []*genai.Part{
		{Text: fullPrompt},
	}

	content := []*genai.Content{
		{Role: "user", Parts: parts},
	}

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", content, nil)
	if err != nil {
		return "", fmt.Errorf("error generating content: %v", err)
	}

	if resp != nil && len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		if resp.Candidates[0].Content.Parts[0].Text != "" {
			return resp.Candidates[0].Content.Parts[0].Text, nil
		}
	}

	return "", fmt.Errorf("no content generated")
}

// handleWebSocket handles WebSocket connections for live audio transcription using Google Cloud Speech-to-Text
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	var mu sync.Mutex // Mutex to protect concurrent writes to the WebSocket connection
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	logger.Info("WebSocket connection established")

	// Read the initial configuration message from the client
	_, p, err := conn.ReadMessage()
	if err != nil {
		logger.Error("Failed to read config message", "error", err)
		return
	}

	var config ConfigMessage
	if err := json.Unmarshal(p, &config); err != nil {
		logger.Error("Failed to unmarshal config message", "error", err)
		return
	}

	logger.Info("Received configuration",
		"audioFormat", config.AudioFormat,
		"languageCode", config.LanguageCode,
		"alternativeLanguageCodes", config.AlternativeLanguageCodes)

	// Debug: Log the exact format string received
	logger.Debug("Exact audio format received", "format", config.AudioFormat.Format)

	ctx := context.Background()

	// Get project ID and location from environment variables
	projectID := os.Getenv("GCP_PROJECT_ID")
	location := os.Getenv("GCP_LOCATION")
	if projectID == "" || location == "" {
		logger.Warn("GCP environment variables not set, summary generation disabled",
			"missing", "GCP_PROJECT_ID or GCP_LOCATION")
	} else {
		logger.Info("GCP configuration loaded",
			"projectID", projectID,
			"location", location)
	}

	// Create Speech-to-Text client
	client, err := speech.NewClient(ctx)
	if err != nil {
		logger.Error("Failed to create Speech-to-Text client", "error", err)
		return
	}
	defer client.Close()

	// Set default language codes if none are provided by the client
	primaryLanguage := config.LanguageCode
	if primaryLanguage == "" {
		primaryLanguage = "en-US" // Default primary language
	}
	alternativeLanguages := config.AlternativeLanguageCodes
	if len(alternativeLanguages) == 0 && primaryLanguage == "en-US" {
		alternativeLanguages = []string{"fr-FR", "es-ES"} // Default alternatives if primary is en-US and no alternatives provided
	}

	logger.Info("Language configuration",
		"primaryLanguage", primaryLanguage,
		"alternativeLanguages", alternativeLanguages)

	// Map audio format string to Google Speech API encoding
	var encoding speechpb.RecognitionConfig_AudioEncoding
	formatLower := strings.ToLower(config.AudioFormat.Format)

	logger.Debug("Mapping audio format to Speech API encoding", "format", config.AudioFormat.Format)

	switch formatLower {
	case "linear16":
		encoding = speechpb.RecognitionConfig_LINEAR16
		logger.Debug("Audio encoding selected", "encoding", "LINEAR16")
	case "ogg_opus":
		encoding = speechpb.RecognitionConfig_OGG_OPUS
		logger.Debug("Audio encoding selected", "encoding", "OGG_OPUS")
	case "webm_opus":
		encoding = speechpb.RecognitionConfig_WEBM_OPUS
		logger.Debug("Audio encoding selected", "encoding", "WEBM_OPUS")
	case "flac":
		encoding = speechpb.RecognitionConfig_FLAC
		logger.Debug("Audio encoding selected", "encoding", "FLAC")
	case "mulaw":
		encoding = speechpb.RecognitionConfig_MULAW
		logger.Debug("Audio encoding selected", "encoding", "MULAW")
	default:
		// Try using the value lookup as fallback
		if encodingValue, exists := speechpb.RecognitionConfig_AudioEncoding_value[config.AudioFormat.Format]; exists {
			encoding = speechpb.RecognitionConfig_AudioEncoding(encodingValue)
			logger.Debug("Audio encoding from value lookup", "encoding", encoding)
		} else {
			logger.Warn("Unknown audio format, defaulting to LINEAR16", "format", config.AudioFormat.Format)
			encoding = speechpb.RecognitionConfig_LINEAR16
		}
	}

	// Configure the streaming recognition request template
	reqTemplate := speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:                 encoding,
					SampleRateHertz:          int32(config.AudioFormat.SampleRate),
					LanguageCode:             primaryLanguage,
					AlternativeLanguageCodes: alternativeLanguages,
				},
				InterimResults: true,
			},
		},
	}

	logger.Info("Speech API configuration finalized",
		"encoding", encoding,
		"sampleRate", config.AudioFormat.SampleRate,
		"language", primaryLanguage)

	// Stream management variables
	var stream speechpb.Speech_StreamingRecognizeClient
	var streamMu sync.Mutex
	streamStartTime := time.Now()
	const maxStreamDuration = 300 * time.Second // 300 seconds, slightly less than 305s limit
	var pendingAudioChunks [][]byte // Buffer for audio chunks during stream recreation
	
	// Function to create or recreate the stream
	createStream := func() error {
		streamMu.Lock()
		defer streamMu.Unlock()
		
		// Close existing stream if it exists
		if stream != nil {
			stream.CloseSend()
		}
		
		// Create a new bidirectional streaming RPC
		newStream, err := client.StreamingRecognize(ctx)
		if err != nil {
			return fmt.Errorf("failed to create streaming client: %v", err)
		}
		
		// Send the initial configuration message
		if err := newStream.Send(&reqTemplate); err != nil {
			return fmt.Errorf("failed to send initial config to Speech-to-Text: %v", err)
		}
		
		stream = newStream
		streamStartTime = time.Now()
		
		// Send any buffered audio chunks
		if len(pendingAudioChunks) > 0 {
			logger.Info("Sending buffered audio chunks after stream recreation", "chunks", len(pendingAudioChunks))
			for _, chunk := range pendingAudioChunks {
				if err := newStream.Send(&speechpb.StreamingRecognizeRequest{
					StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
						AudioContent: chunk,
					},
				}); err != nil {
					logger.Error("Failed to send buffered audio chunk", "error", err)
					break
				}
			}
			// Clear the buffer after sending
			pendingAudioChunks = nil
		}
		
		logger.Info("Speech-to-Text stream created/recreated")
		
		// Notify client about stream recreation
		statusResponse := StatusResponse{
			Type:      "status",
			Status:    "stream_recreated",
			Message:   "Speech recognition stream was recreated for optimal performance",
			Timestamp: time.Now(),
		}
		statusData, _ := json.Marshal(statusResponse)
		// Send status update in a goroutine to avoid blocking
		go func() {
			mu.Lock()
			conn.WriteMessage(websocket.TextMessage, statusData)
			mu.Unlock()
		}()
		
		return nil
	}
	
	// Create initial stream
	if err := createStream(); err != nil {
		logger.Error("Failed to create initial stream", "error", err)
		return
	}

	var fullTranscription strings.Builder
	var currentSummary string
	var summaryMu sync.Mutex // Protect currentSummary from race conditions

	// Default prompt for summarization
	defaultSummaryPrompt := `You are tasked with creating and maintaining a summary of a live conversation transcript. Follow these guidelines:

1. **Language**: Write the summary in the same language as the majority of the transcript
2. **Iterative approach**: Keep the initial summary as much as possible and only make changes if there are inconsistencies, nonsensical parts, or incoherent content
3. **Completion**: Simply complete or extend the summary with new information from the transcript
4. **Accuracy**: Do not invent or add information that is not present in the transcript
5. **Important quotes**: When something is particularly important, include a direct quote from the transcript
6. **Format**: Use markdown formatting for better readability. Put emphasis (bold and italic) on important concept, and use > for quotes.

If this is an update to an existing summary, maintain the structure and content of the previous summary unless corrections are needed.`

	// Get summarization prompt from environment variable, or use default
	summaryPrompt := os.Getenv("SUMMARY_PROMPT")
	if summaryPrompt == "" {
		summaryPrompt = defaultSummaryPrompt
	}

	// Goroutine to receive messages from Speech-to-Text and send to client
	go func() {
		for {
			var currentStream speechpb.Speech_StreamingRecognizeClient
			
			// Get current stream reference safely
			streamMu.Lock()
			currentStream = stream
			streamMu.Unlock()
			
			if currentStream == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			
			resp, err := currentStream.Recv()
			if err == io.EOF {
				// Stream closed, try to recreate
				logger.Debug("Speech-to-Text stream closed, recreating...")
				if recreateErr := createStream(); recreateErr != nil {
					logger.Error("Failed to recreate stream", "error", recreateErr)
					return
				}
				// After recreation, continue to get new stream reference
				continue
			}
			if err != nil {
				logger.Error("Error receiving from Speech-to-Text", "error", err)
				// Try to recreate stream on error
				if recreateErr := createStream(); recreateErr != nil {
					logger.Error("Failed to recreate stream after error", "error", recreateErr)
					return
				}
				// After recreation, continue to get new stream reference
				continue
			}

			if err := resp.Error; err != nil {
				logger.Error("Speech-to-Text API error", "error", err)
				continue
			}

			for _, result := range resp.Results {
				if len(result.Alternatives) > 0 {
					transcriptionText := result.Alternatives[0].Transcript
					logger.Debug("Transcription received",
						"text", transcriptionText,
						"isFinal", result.IsFinal)

					response := TranscriptionResponse{
						Type:      "transcription",
						Text:      transcriptionText,
						Timestamp: time.Now(),
						Final:     result.IsFinal,
					}

					responseData, err := json.Marshal(response)
					if err != nil {
						logger.Error("Failed to marshal transcription response", "error", err)
						continue
					}

					mu.Lock()
					if err := conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
						logger.Error("Failed to send transcription to client", "error", err)
						mu.Unlock()
						return
					}
					mu.Unlock()

					if result.IsFinal {
						fullTranscription.WriteString(transcriptionText + " ")
						// Generate summary asynchronously to avoid blocking transcript processing
						if projectID != "" && location != "" {
							go func() {
								fullTranscript := strings.TrimSpace(fullTranscription.String())

								// Safely read current summary
								summaryMu.Lock()
								previousSummary := currentSummary
								summaryMu.Unlock()

								logger.Debug("Generating summary",
									"transcriptLength", len(fullTranscript),
									"previousSummaryLength", len(previousSummary))
								summary, err := generateSummary(ctx, projectID, location, fullTranscript, previousSummary, summaryPrompt)
								if err != nil {
									logger.Error("Error generating summary", "error", err)
									return
								}
								if summary != "" {
									// Safely update current summary
									summaryMu.Lock()
									currentSummary = summary
									summaryMu.Unlock()

									logger.Info("Summary generated", "summaryLength", len(summary))
									summaryResponse := SummaryResponse{
										Type:      "summary",
										Text:      summary,
										Timestamp: time.Now(),
									}
									summaryData, err := json.Marshal(summaryResponse)
									if err != nil {
										logger.Error("Failed to marshal summary response", "error", err)
										return
									}
									mu.Lock()
									if err := conn.WriteMessage(websocket.TextMessage, summaryData); err != nil {
										logger.Error("Failed to send summary to client", "error", err)
									}
									mu.Unlock()
								}
							}()
						}
					}
				}
			}
		}
	}()
	
	// Goroutine to monitor stream duration and restart before hitting the limit
	go func() {
		ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				streamMu.Lock()
				elapsed := time.Since(streamStartTime)
				streamMu.Unlock()
				
				if elapsed >= maxStreamDuration {
					logger.Info("Stream duration limit approaching, recreating stream",
						"elapsed", elapsed,
						"limit", maxStreamDuration)
					if err := createStream(); err != nil {
						logger.Error("Failed to recreate stream due to duration limit", "error", err)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Main loop to read from client and send audio to Speech-to-Text
	audioChunkCount := 0
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error("Unexpected WebSocket error", "error", err)
			}
			break
		}

		switch messageType {
		case websocket.BinaryMessage:
			audioChunkCount++
			logger.Debug("Received audio chunk",
				"chunkNumber", audioChunkCount,
				"bytes", len(message))

			// Send audio content to Speech-to-Text
			streamMu.Lock()
			currentStream := stream
			streamMu.Unlock()
			
			if currentStream != nil {
				if err := currentStream.Send(&speechpb.StreamingRecognizeRequest{
					StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
						AudioContent: message,
					},
				}); err != nil {
					logger.Error("Failed to send audio chunk to Speech-to-Text",
						"chunkNumber", audioChunkCount,
						"error", err)
					
					// Buffer this audio chunk before recreating stream
					streamMu.Lock()
					pendingAudioChunks = append(pendingAudioChunks, message)
					// Limit buffer size to prevent memory issues
					if len(pendingAudioChunks) > 10 {
						pendingAudioChunks = pendingAudioChunks[1:] // Remove oldest chunk
					}
					streamMu.Unlock()
					
					// Try to recreate stream on send error
					if recreateErr := createStream(); recreateErr != nil {
						logger.Error("Failed to recreate stream after send error", "error", recreateErr)
						return
					}
					continue
				}
			} else {
				// Stream is nil, buffer the audio chunk
				streamMu.Lock()
				pendingAudioChunks = append(pendingAudioChunks, message)
				// Limit buffer size to prevent memory issues
				if len(pendingAudioChunks) > 10 {
					pendingAudioChunks = pendingAudioChunks[1:] // Remove oldest chunk
				}
				streamMu.Unlock()
				logger.Debug("Buffered audio chunk (stream is nil)", "chunkNumber", audioChunkCount)
			}
			logger.Debug("Successfully processed audio chunk",
				"chunkNumber", audioChunkCount)
		case websocket.TextMessage:
			logger.Debug("Received text message", "message", string(message))
			// Check if it's a new config message (for system audio mode)
			var newConfig ConfigMessage
			if err := json.Unmarshal(message, &newConfig); err == nil && newConfig.Type == "config" {
				logger.Info("Received new config message", "config", newConfig)
			}
		}
	}

	// Close the Speech-to-Text stream when the WebSocket connection closes
	streamMu.Lock()
	if stream != nil {
		stream.CloseSend()
	}
	streamMu.Unlock()
	logger.Info("WebSocket connection closed")
}

// serveStaticFiles serves static HTML and JS files
func serveStaticFiles(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		http.ServeFile(w, r, "live_transcription_ui.html")
	case "/live_audio_recorder.js":
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "live_audio_recorder.js")
	default:
		http.NotFound(w, r)
	}
}

func main() {
	// Initialize logging
	initLogger()

	// Set up routes
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/", serveStaticFiles)

	// Start server
	port := ":8080"
	logger.Info("Starting server",
		"address", fmt.Sprintf("http://localhost%s", port),
		"websocket", fmt.Sprintf("ws://localhost%s/ws", port))

	if err := http.ListenAndServe(port, nil); err != nil {
		logger.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
}
