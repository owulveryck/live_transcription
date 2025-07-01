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

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin for development
	},
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
	var currentSummary string

	// Default prompt for summarization
	defaultSummaryPrompt := `You are tasked with creating and maintaining a summary of a live conversation transcript. Follow these guidelines:

1. **Language**: Write the summary in the same language as the majority of the transcript
2. **Iterative approach**: Keep the initial summary as much as possible and only make changes if there are inconsistencies, nonsensical parts, or incoherent content
3. **Completion**: Simply complete or extend the summary with new information from the transcript
4. **Accuracy**: Do not invent or add information that is not present in the transcript
5. **Important quotes**: When something is particularly important, include a direct quote from the transcript
6. **Format**: Use markdown formatting for better readability

If this is an update to an existing summary, maintain the structure and content of the previous summary unless corrections are needed.`

	// Get summarization prompt from environment variable, or use default
	summaryPrompt := os.Getenv("SUMMARY_PROMPT")
	if summaryPrompt == "" {
		summaryPrompt = defaultSummaryPrompt
	}

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
							fullTranscript := strings.TrimSpace(fullTranscription.String())
							log.Printf("Attempting to generate summary for transcript length: %d, previous summary length: %d", len(fullTranscript), len(currentSummary))
							summary, err := generateSummary(ctx, projectID, location, fullTranscript, currentSummary, summaryPrompt)
							if err != nil {
								log.Printf("Error generating summary: %v", err)
							} else if summary != "" {
								currentSummary = summary // Update current summary
								log.Printf("Generated updated summary: %s", summary)
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

