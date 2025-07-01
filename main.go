package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	speech "cloud.google.com/go/speech/apiv1"
	"cloud.google.com/go/vertexai/genai"
	"github.com/gorilla/websocket"
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

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin for development
	},
}

// generateWithText uses Vertex AI to generate content based on the provided text and prompt
func generateWithText(ctx context.Context, projectID, location, text, prompt string) (string, error) {
	if text == "" {
		return "", nil
	}

	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		return "", fmt.Errorf("error creating Vertex AI client: %v", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-2.5-flash")

	fullPrompt := fmt.Sprintf("%s\n\n%s", prompt, text)

	resp, err := model.GenerateContent(ctx, genai.Text(fullPrompt))
	if err != nil {
		return "", fmt.Errorf("error generating content: %v", err)
	}

	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		if generatedText, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
			return string(generatedText), nil
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
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Println("WebSocket connection established")

	// Read the initial configuration message from the client
	_, p, err := conn.ReadMessage()
	if err != nil {
		log.Printf("Failed to read config message: %v", err)
		return
	}

	var config ConfigMessage
	if err := json.Unmarshal(p, &config); err != nil {
		log.Printf("Failed to unmarshal config message: %v", err)
		return
	}

	log.Printf("Received config: AudioFormat=%+v, LanguageCode=%s, AlternativeLanguageCodes=%v", config.AudioFormat, config.LanguageCode, config.AlternativeLanguageCodes)

	ctx := context.Background()

	// Get project ID and location from environment variables
	projectID := os.Getenv("GCP_PROJECT_ID")
	location := os.Getenv("GCP_LOCATION")
	if projectID == "" || location == "" {
		log.Println("GCP_PROJECT_ID or GCP_LOCATION environment variable not set. Summary generation will be disabled.")
	} else {
		log.Printf("GCP_PROJECT_ID: %s, GCP_LOCATION: %s", projectID, location)
	}

	// Create Speech-to-Text client
	client, err := speech.NewClient(ctx)
	if err != nil {
		log.Printf("Failed to create Speech-to-Text client: %v", err)
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

	log.Printf("Using primary language: %s, alternative languages: %v", primaryLanguage, alternativeLanguages)

	// Configure the streaming recognition request
	req := speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:                 speechpb.RecognitionConfig_AudioEncoding(speechpb.RecognitionConfig_AudioEncoding_value[config.AudioFormat.Format]),
					SampleRateHertz:          int32(config.AudioFormat.SampleRate),
					LanguageCode:             primaryLanguage,
					AlternativeLanguageCodes: alternativeLanguages,
				},
				InterimResults: true,
			},
		},
	}

	// Create a bidirectional streaming RPC
	stream, err := client.StreamingRecognize(ctx)
	if err != nil {
		log.Printf("Failed to create streaming client: %v", err)
		return
	}

	// Send the initial configuration message
	if err := stream.Send(&req); err != nil {
		log.Printf("Failed to send initial config to Speech-to-Text: %v", err)
		return
	}

	log.Println("Connected to Google Cloud Speech-to-Text streaming session")

	var fullTranscription strings.Builder

	// Goroutine to receive messages from Speech-to-Text and send to client
	go func() {
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				// Stream closed
				return
			}
			if err != nil {
				log.Printf("Error receiving from Speech-to-Text: %v", err)
				return
			}

			if err := resp.Error; err != nil {
				log.Printf("Speech-to-Text error: %v", err)
				continue
			}

			for _, result := range resp.Results {
				if len(result.Alternatives) > 0 {
					transcriptionText := result.Alternatives[0].Transcript
					log.Printf("Transcription: %s (is_final: %t)", transcriptionText, result.IsFinal)

					response := TranscriptionResponse{
						Type:      "transcription",
						Text:      transcriptionText,
						Timestamp: time.Now(),
						Final:     result.IsFinal,
					}

					responseData, err := json.Marshal(response)
					if err != nil {
						log.Printf("Failed to marshal transcription response: %v", err)
						continue
					}

					mu.Lock()
					if err := conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
						log.Printf("Failed to send transcription to client: %v", err)
						mu.Unlock()
						return
					}
					mu.Unlock()

					if result.IsFinal {
						fullTranscription.WriteString(transcriptionText + " ")
						if projectID != "" && location != "" {
							log.Printf("Attempting to generate summary for text length: %d, content: %s", fullTranscription.Len(), fullTranscription.String())
							summaryPrompt := "Summarize the following text:"
							summary, err := generateWithText(ctx, projectID, location, fullTranscription.String(), summaryPrompt)
							if err != nil {
								log.Printf("Error generating summary: %v", err)
							} else {
								log.Printf("Generated summary: %s", summary)
							}
							if err != nil {
								log.Printf("Error generating summary: %v", err)
							} else if summary != "" {
								summaryResponse := SummaryResponse{
									Type:      "summary",
									Text:      summary,
									Timestamp: time.Now(),
								}
								summaryData, err := json.Marshal(summaryResponse)
								if err != nil {
									log.Printf("Failed to marshal summary response: %v", err)
								} else {
									mu.Lock()
									if err := conn.WriteMessage(websocket.TextMessage, summaryData); err != nil {
										log.Printf("Failed to send summary to client: %v", err)
									}
									mu.Unlock()
								}
							}
						}
					}
				}
			}
		}
	}()

	// Main loop to read from client and send audio to Speech-to-Text
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		switch messageType {
		case websocket.BinaryMessage:
			// Send audio content to Speech-to-Text
			if err := stream.Send(&speechpb.StreamingRecognizeRequest{
				StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
					AudioContent: message,
				},
			}); err != nil {
				log.Printf("Failed to send audio to Speech-to-Text: %v", err)
				return
			}
		case websocket.TextMessage:
			log.Printf("Received text message (ignoring for now): %s", string(message))
		}
	}

	// Close the Speech-to-Text stream when the WebSocket connection closes
	stream.CloseSend()
	log.Println("WebSocket connection closed")
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
	// Set up routes
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/", serveStaticFiles)

	// Start server
	port := ":8080"
	log.Printf("Starting server on http://localhost%s", port)
	log.Printf("WebSocket endpoint: ws://localhost%s/ws", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

