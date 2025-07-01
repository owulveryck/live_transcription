package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/genai"
)

// AudioFormat represents the audio stream format
type AudioFormat struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

// ConfigMessage represents the initial configuration sent from the client
type ConfigMessage struct {
	Type          string `json:"type"`
	SummaryPrompt string `json:"summaryPrompt"`
	RecordingMode string `json:"recordingMode"`
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
	Type string `json:"type"`
	Text string `json:"text"`
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow connections from any origin for development
	},
}

// handleWebSocket handles WebSocket connections for live audio transcription using Vertex AI
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
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

	log.Printf("Received config: SummaryPrompt=\"%s\", RecordingMode=\"%s\"", config.SummaryPrompt, config.RecordingMode)

	ctx := context.Background()

	// Create Vertex AI client
	client, err := genai.NewClient(ctx, &genai.ClientConfig{Backend: genai.BackendVertexAI})
	if err != nil {
		log.Printf("Failed to create Vertex AI client: %v", err)
		return
	}

	model := "gemini-2.0-flash-live-preview-04-09"

	// Establish live session with Vertex AI, using the provided summary prompt as SystemInstruction
	session, err := client.Live.Connect(ctx, model, &genai.LiveConnectConfig{
		InputAudioTranscription:  &genai.AudioTranscriptionConfig{},
		OutputAudioTranscription: &genai.AudioTranscriptionConfig{},
		ResponseModalities:       []genai.Modality{genai.ModalityText},
		SystemInstruction:        genai.Text(config.SummaryPrompt)[0],
	})


	if err != nil {
		log.Printf("Failed to connect to Vertex AI model: %v", err)
		return
	}
	defer session.Close()

	log.Println("Connected to Vertex AI live session")

	// Goroutine to receive messages from Vertex AI and send to client
	go func() {
		for {
			message, err := session.Receive()
			if err != nil {
				log.Printf("Error receiving from Vertex AI: %v", err)
				return
			}

			// Process transcription responses
			if message.ServerContent != nil && message.ServerContent.InputTranscription != nil {
				transcriptionText := message.ServerContent.InputTranscription.Text
				log.Printf("Transcription: %s", transcriptionText)

				log.Println(transcriptionText)
				// Create response for frontend
				response := TranscriptionResponse{
					Type:      "transcription",
					Text:      transcriptionText,
					Timestamp: time.Now(),
					Final:     true,
				}

				responseData, err := json.Marshal(response)
				if err != nil {
					log.Printf("Failed to marshal transcription response: %v", err)
					continue
				}

				if err := conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
					log.Printf("Failed to send transcription to client: %v", err)
					return
				}
			} else if message.ServerContent != nil {
				// Attempt to extract text from generic ServerContent
				var genericContent map[string]interface{}
				contentBytes, err := json.Marshal(message.ServerContent)
				if err != nil {
					log.Printf("Failed to marshal ServerContent to generic map: %v", err)
					continue
				}
				if err := json.Unmarshal(contentBytes, &genericContent); err != nil {
					log.Printf("Failed to unmarshal ServerContent to generic map: %v", err)
					continue
				}

				modelOutputText := ""
				if candidates, ok := genericContent["candidates"].([]interface{}); ok && len(candidates) > 0 {
					if candidateMap, ok := candidates[0].(map[string]interface{}); ok {
						if parts, ok := candidateMap["parts"].([]interface{}); ok && len(parts) > 0 {
							if partMap, ok := parts[0].(map[string]interface{}); ok {
								if text, ok := partMap["text"].(string); ok {
									modelOutputText = text
								}
							}
						}
					}
				} else if text, ok := genericContent["text"].(string); ok {
					modelOutputText = text
				}

				if modelOutputText != "" {
					log.Printf("Model Output (generic): %s", modelOutputText)

					response := SummaryResponse{
						Type: "summary",
						Text: modelOutputText,
					}

					responseData, err := json.Marshal(response)
					if err != nil {
						log.Printf("Failed to marshal summary response (generic): %v", err)
						continue
					}

					if err := conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
						log.Printf("Failed to send summary to client (generic): %v", err)
						return
					}
				}
			}

			// Log the entire message for debugging
			messageBytes, err := json.Marshal(message)
			if err != nil {
				log.Printf("Failed to marshal Vertex AI response for debugging: %v", err)
				continue
			}
			log.Printf("Full Vertex AI message: %s", messageBytes)
		}
	}()

	// Main loop to read from client and send to Vertex AI
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		switch messageType {
		case websocket.TextMessage:
			// Try to parse as LiveRealtimeInput for Vertex AI
			var realtimeInput genai.LiveRealtimeInput
			if err := json.Unmarshal(message, &realtimeInput); err != nil {
				log.Printf("Failed to parse realtime input: %v", err)
				continue
			}

			// Send to Vertex AI
			session.SendRealtimeInput(realtimeInput)
			// log.Printf("Sent realtime input to Vertex AI")

		case websocket.BinaryMessage:
			log.Printf("Received binary message: %d bytes (ignoring for now)", len(message))
			// Binary messages are not expected in this Vertex AI implementation
			// The frontend should send JSON messages only
		}
	}

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
