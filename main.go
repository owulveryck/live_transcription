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
	"sync/atomic"
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

// PhraseSetConfig represents phrase sets configuration from the client
type PhraseSetConfig struct {
	Phrases []PhraseItem `json:"phrases"`
}

// PhraseItem represents a phrase with boost value
type PhraseItem struct {
	Value string  `json:"value"`
	Boost float32 `json:"boost"`
}

// CustomClass represents a single custom class with its items and boost
type CustomClass struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
	Boost float32  `json:"boost"`
}

// ClassesConfig represents classes configuration from the client
type ClassesConfig struct {
	PredefinedClasses []string      `json:"predefinedClasses"`
	CustomClasses     []CustomClass `json:"customClasses"`
	// Legacy support for single custom class
	CustomClassItems  []string  `json:"customClassItems,omitempty"`
	Boost             float32   `json:"boost,omitempty"`
}

// ConfigMessage represents the initial configuration sent from the client
type ConfigMessage struct {
	Type                     string           `json:"type"`
	AudioFormat              AudioFormat      `json:"audioFormat"`
	LanguageCode             string           `json:"languageCode"`
	AlternativeLanguageCodes []string         `json:"alternativeLanguageCodes"`
	CustomWords              []string         `json:"customWords"`
	PhraseSets               *PhraseSetConfig `json:"phraseSets"`
	Classes                  *ClassesConfig   `json:"classes"`
	SummaryPrompt            string           `json:"summaryPrompt,omitempty"`
}

// KeywordsMessage represents keywords sent from the client during an active session
type KeywordsMessage struct {
	Type      string    `json:"type"`
	Words     []string  `json:"words"`
	Timestamp time.Time `json:"timestamp"`
}

// EndPromptMessage represents an end prompt sent from the client when stopping
type EndPromptMessage struct {
	Type      string    `json:"type"`
	EndPrompt string    `json:"endPrompt"`
	Timestamp time.Time `json:"timestamp"`
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

// Global variables for dynamic keyword management
var keywordsMu sync.Mutex
var currentSpeechContexts []*speechpb.SpeechContext
var dynamicKeywords []string

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

// createSpeechContexts creates speech contexts with custom words/phrases for enhanced recognition
func createSpeechContexts(customWords []string) []*speechpb.SpeechContext {
	if len(customWords) == 0 {
		return nil
	}

	// Create phrases from custom words
	var phrases []string
	for _, word := range customWords {
		if strings.TrimSpace(word) != "" {
			phrases = append(phrases, strings.TrimSpace(word))
		}
	}

	if len(phrases) == 0 {
		return nil
	}

	logger.Debug("Creating SpeechContext", 
		"phrasesCount", len(phrases),
		"customWords", customWords)

	// Create a speech context with the custom phrases
	speechContext := &speechpb.SpeechContext{
		Phrases: phrases,
		Boost:   10.0, // Boost recognition confidence for these phrases
	}

	logger.Info("SpeechContext created successfully", "phrasesCount", len(phrases))

	return []*speechpb.SpeechContext{speechContext}
}

// createDynamicSpeechContexts creates updated speech contexts by combining original contexts with new dynamic keywords
func createDynamicSpeechContexts(originalContexts []*speechpb.SpeechContext, newKeywords []string) []*speechpb.SpeechContext {
	if len(newKeywords) == 0 {
		logger.Debug("No new keywords provided, returning original contexts")
		return originalContexts
	}

	logger.Info("Creating dynamic SpeechContexts",
		"originalContextsCount", len(originalContexts),
		"newKeywordsCount", len(newKeywords),
		"newKeywords", newKeywords)

	// Create a copy of original contexts
	updatedContexts := make([]*speechpb.SpeechContext, len(originalContexts))
	copy(updatedContexts, originalContexts)

	// Filter and prepare new keywords
	var validKeywords []string
	for i, keyword := range newKeywords {
		trimmedKeyword := strings.TrimSpace(keyword)
		logger.Debug("Processing dynamic keyword",
			"index", i+1,
			"originalKeyword", keyword,
			"trimmedKeyword", trimmedKeyword,
			"isEmpty", trimmedKeyword == "")

		if trimmedKeyword != "" {
			validKeywords = append(validKeywords, trimmedKeyword)
			logger.Debug("Dynamic keyword accepted",
				"validIndex", len(validKeywords),
				"keyword", trimmedKeyword)
		}
	}

	// Add new keywords as a separate speech context if we have valid ones
	if len(validKeywords) > 0 {
		dynamicContext := &speechpb.SpeechContext{
			Phrases: validKeywords,
			Boost:   15.0, // Higher boost for dynamic keywords to prioritize them
		}
		updatedContexts = append(updatedContexts, dynamicContext)

		logger.Info("Dynamic SpeechContext created",
			"validKeywordsCount", len(validKeywords),
			"boost", 15.0,
			"totalContextsAfterUpdate", len(updatedContexts))
	}

	logger.Info("Dynamic SpeechContexts creation completed",
		"finalContextsCount", len(updatedContexts),
		"addedDynamicContext", len(validKeywords) > 0)

	return updatedContexts
}

// createAdvancedSpeechContexts creates advanced speech contexts with phrase sets and classes
func createAdvancedSpeechContexts(customWords []string, phraseSetsConfig *PhraseSetConfig, classesConfig *ClassesConfig) []*speechpb.SpeechContext {
	var speechContexts []*speechpb.SpeechContext

	// Handle custom words (legacy support)
	if len(customWords) > 0 {
		contexts := createSpeechContexts(customWords)
		speechContexts = append(speechContexts, contexts...)
	}

	// Handle phrase sets configuration
	if phraseSetsConfig != nil && len(phraseSetsConfig.Phrases) > 0 {
		logger.Info("Processing phrase sets configuration",
			"totalPhraseItems", len(phraseSetsConfig.Phrases))

		var phrases []string
		var totalBoostSum float32
		var validPhraseCount int

		for i, phraseItem := range phraseSetsConfig.Phrases {
			trimmedPhrase := strings.TrimSpace(phraseItem.Value)
			logger.Debug("Processing phrase set item",
				"index", i+1,
				"originalPhrase", phraseItem.Value,
				"trimmedPhrase", trimmedPhrase,
				"boost", phraseItem.Boost,
				"isEmpty", trimmedPhrase == "")

			if trimmedPhrase != "" {
				phrases = append(phrases, trimmedPhrase)
				totalBoostSum += phraseItem.Boost
				validPhraseCount++
				logger.Debug("Phrase set item accepted",
					"validIndex", validPhraseCount,
					"phrase", trimmedPhrase,
					"boost", phraseItem.Boost)
			} else {
				logger.Debug("Phrase set item skipped (empty after trim)",
					"index", i+1,
					"originalValue", phraseItem.Value)
			}
		}

		if len(phrases) > 0 {
			averageBoost := totalBoostSum / float32(validPhraseCount)
			logger.Info("Creating SpeechContext from phrase sets",
				"validPhrasesCount", len(phrases),
				"skippedPhrasesCount", len(phraseSetsConfig.Phrases)-validPhraseCount,
				"averageBoost", averageBoost,
				"usingDefaultBoost", 10.0)

			speechContext := &speechpb.SpeechContext{
				Phrases: phrases,
				Boost:   10.0, // Default boost for phrase sets
			}
			speechContexts = append(speechContexts, speechContext)
			logger.Info("PhraseSet SpeechContext created successfully",
				"phrasesCount", len(phrases),
				"phrases", phrases,
				"boost", 10.0)
		} else {
			logger.Warn("No valid phrases found in phrase sets configuration",
				"totalItems", len(phraseSetsConfig.Phrases),
				"allItemsEmpty", true)
		}
	} else {
		logger.Debug("No phrase sets configuration provided or phrase sets is empty")
	}

	// Handle classes configuration
	if classesConfig != nil {
		var classHints []string

		// Add predefined classes
		for _, class := range classesConfig.PredefinedClasses {
			if strings.TrimSpace(class) != "" {
				classHints = append(classHints, strings.TrimSpace(class))
			}
		}

		// Handle multiple custom classes (new format)
		if len(classesConfig.CustomClasses) > 0 {
			logger.Info("Processing custom classes configuration",
				"totalCustomClasses", len(classesConfig.CustomClasses))

			for classIndex, customClass := range classesConfig.CustomClasses {
				logger.Info("Processing custom class",
					"classIndex", classIndex+1,
					"className", customClass.Name,
					"totalItems", len(customClass.Items),
					"boost", customClass.Boost)

				var customClassPhrases []string
				for itemIndex, item := range customClass.Items {
					trimmedItem := strings.TrimSpace(item)
					logger.Debug("Processing custom class item",
						"classIndex", classIndex+1,
						"className", customClass.Name,
						"itemIndex", itemIndex+1,
						"originalItem", item,
						"trimmedItem", trimmedItem,
						"isEmpty", trimmedItem == "")

					if trimmedItem != "" {
						customClassPhrases = append(customClassPhrases, trimmedItem)
						logger.Debug("Custom class item accepted",
							"className", customClass.Name,
							"validItemIndex", len(customClassPhrases),
							"item", trimmedItem)
					} else {
						logger.Debug("Custom class item skipped (empty after trim)",
							"className", customClass.Name,
							"itemIndex", itemIndex+1,
							"originalValue", item)
					}
				}

				if len(customClassPhrases) > 0 {
					logger.Info("Creating SpeechContext from custom class",
						"className", customClass.Name,
						"validItemsCount", len(customClassPhrases),
						"skippedItemsCount", len(customClass.Items)-len(customClassPhrases),
						"boost", customClass.Boost,
						"items", customClassPhrases)

					speechContext := &speechpb.SpeechContext{
						Phrases: customClassPhrases,
						Boost:   customClass.Boost,
					}
					speechContexts = append(speechContexts, speechContext)
					logger.Info("Custom class SpeechContext created successfully",
						"className", customClass.Name,
						"itemsCount", len(customClassPhrases),
						"boost", customClass.Boost,
						"speechContextIndex", len(speechContexts))
				} else {
					logger.Warn("Custom class has no valid items, skipping SpeechContext creation",
						"className", customClass.Name,
						"totalItems", len(customClass.Items),
						"allItemsEmpty", true)
				}
			}
		} else if len(classesConfig.CustomClassItems) > 0 {
			// Legacy support for single custom class
			logger.Info("Processing legacy custom class items",
				"totalItems", len(classesConfig.CustomClassItems),
				"boost", classesConfig.Boost)

			var customClassPhrases []string
			for itemIndex, item := range classesConfig.CustomClassItems {
				trimmedItem := strings.TrimSpace(item)
				logger.Debug("Processing legacy custom class item",
					"itemIndex", itemIndex+1,
					"originalItem", item,
					"trimmedItem", trimmedItem,
					"isEmpty", trimmedItem == "")

				if trimmedItem != "" {
					customClassPhrases = append(customClassPhrases, trimmedItem)
					logger.Debug("Legacy custom class item accepted",
						"validItemIndex", len(customClassPhrases),
						"item", trimmedItem)
				} else {
					logger.Debug("Legacy custom class item skipped (empty after trim)",
						"itemIndex", itemIndex+1,
						"originalValue", item)
				}
			}

			if len(customClassPhrases) > 0 {
				logger.Info("Creating SpeechContext from legacy custom class items",
					"validItemsCount", len(customClassPhrases),
					"skippedItemsCount", len(classesConfig.CustomClassItems)-len(customClassPhrases),
					"boost", classesConfig.Boost,
					"items", customClassPhrases)

				speechContext := &speechpb.SpeechContext{
					Phrases: customClassPhrases,
					Boost:   classesConfig.Boost,
				}
				speechContexts = append(speechContexts, speechContext)
				logger.Info("Legacy custom class SpeechContext created successfully",
					"itemsCount", len(customClassPhrases),
					"boost", classesConfig.Boost,
					"speechContextIndex", len(speechContexts))
			} else {
				logger.Warn("Legacy custom class has no valid items, skipping SpeechContext creation",
					"totalItems", len(classesConfig.CustomClassItems),
					"allItemsEmpty", true)
			}
		} else {
			logger.Debug("No custom class items (legacy or new format) provided")
		}

		// Add predefined classes as phrases with boost (use first custom class boost or legacy boost)
		if len(classHints) > 0 {
			defaultBoost := classesConfig.Boost
			if len(classesConfig.CustomClasses) > 0 {
				defaultBoost = classesConfig.CustomClasses[0].Boost
				logger.Debug("Using boost from first custom class for predefined classes",
					"firstCustomClassName", classesConfig.CustomClasses[0].Name,
					"boost", defaultBoost)
			} else {
				logger.Debug("Using legacy boost for predefined classes",
					"boost", defaultBoost)
			}

			logger.Info("Creating SpeechContext from predefined classes",
				"classesCount", len(classHints),
				"boost", defaultBoost,
				"classes", classHints)

			speechContext := &speechpb.SpeechContext{
				Phrases: classHints,
				Boost:   defaultBoost,
			}
			speechContexts = append(speechContexts, speechContext)
			logger.Info("Predefined classes SpeechContext created successfully",
				"classesCount", len(classHints),
				"boost", defaultBoost,
				"speechContextIndex", len(speechContexts))
		} else {
			logger.Debug("No predefined classes to process")
		}
	}

	// Log final summary of speech contexts creation
	if len(speechContexts) > 0 {
		logger.Info("Advanced SpeechContexts creation completed",
			"totalContexts", len(speechContexts),
			"hasCustomWords", len(customWords) > 0,
			"hasPhraseSets", phraseSetsConfig != nil,
			"hasClasses", classesConfig != nil)

		// Log each context summary
		for i, context := range speechContexts {
			logger.Debug("SpeechContext summary",
				"contextIndex", i+1,
				"phrasesCount", len(context.Phrases),
				"boost", context.Boost,
				"firstFewPhrases", func() []string {
					if len(context.Phrases) <= 3 {
						return context.Phrases
					}
					return context.Phrases[:3]
				}())
		}
	} else {
		logger.Info("No SpeechContexts created",
			"customWordsProvided", len(customWords) > 0,
			"phraseSetsProvided", phraseSetsConfig != nil,
			"classesProvided", classesConfig != nil,
			"reason", "All configurations were empty or invalid")
	}

	return speechContexts
}

// generateSummary uses Google GenAI to generate content based on the provided transcript, previous summary, prompt, and custom words
func generateSummary(ctx context.Context, projectID, location, fullTranscript, previousSummary, prompt string, customWords []string) (string, error) {
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

	// Build the full prompt with transcript, previous summary, and custom words
	var fullPrompt string
	customWordsText := ""
	if len(customWords) > 0 {
		customWordsText = fmt.Sprintf("\n\n--- IMPORTANT TERMS/PHRASES ---\nPay special attention to these key terms that appeared in the conversation: %s", strings.Join(customWords, ", "))
	}

	if previousSummary != "" {
		fullPrompt = fmt.Sprintf("%s%s\n\n--- PREVIOUS SUMMARY ---\n%s\n\n--- FULL TRANSCRIPT ---\n%s", prompt, customWordsText, previousSummary, fullTranscript)
	} else {
		fullPrompt = fmt.Sprintf("%s%s\n\n--- FULL TRANSCRIPT ---\n%s", prompt, customWordsText, fullTranscript)
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

	// Create a context that can be cancelled when the WebSocket closes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Read the initial configuration message from the client
	_, p, err := conn.ReadMessage()
	if err != nil {
		logger.Error("Failed to read config message", "error", err)
		cancel() // Cancel context on error
		return
	}

	var config ConfigMessage
	if err := json.Unmarshal(p, &config); err != nil {
		logger.Error("Failed to unmarshal config message", "error", err)
		cancel() // Cancel context on error
		return
	}

	// Log detailed configuration information
	logger.Info("Received configuration",
		"audioFormat", config.AudioFormat,
		"languageCode", config.LanguageCode,
		"alternativeLanguageCodes", config.AlternativeLanguageCodes,
		"customWordsCount", len(config.CustomWords),
		"hasPhraseSetsConfig", config.PhraseSets != nil,
		"hasClassesConfig", config.Classes != nil)

	// Log custom words if present
	if len(config.CustomWords) > 0 {
		logger.Info("Custom words configuration received",
			"words", config.CustomWords,
			"count", len(config.CustomWords))
		for i, word := range config.CustomWords {
			logger.Debug("Custom word detail",
				"index", i+1,
				"word", word,
				"length", len(word))
		}
	}

	// Log phrase sets configuration if present
	if config.PhraseSets != nil {
		logger.Info("Phrase sets configuration received",
			"phrasesCount", len(config.PhraseSets.Phrases))
		for i, phrase := range config.PhraseSets.Phrases {
			logger.Info("Phrase set item",
				"index", i+1,
				"phrase", phrase.Value,
				"boost", phrase.Boost,
				"phraseLength", len(phrase.Value))
		}
	} else {
		logger.Debug("No phrase sets configuration provided")
	}

	// Log classes configuration if present
	if config.Classes != nil {
		logger.Info("Classes configuration received",
			"predefinedClassesCount", len(config.Classes.PredefinedClasses),
			"customClassesCount", len(config.Classes.CustomClasses),
			"hasLegacyCustomClassItems", len(config.Classes.CustomClassItems) > 0,
			"legacyBoost", config.Classes.Boost)

		// Log predefined classes
		if len(config.Classes.PredefinedClasses) > 0 {
			logger.Info("Predefined classes",
				"classes", config.Classes.PredefinedClasses)
			for i, class := range config.Classes.PredefinedClasses {
				logger.Debug("Predefined class detail",
					"index", i+1,
					"class", class)
			}
		}

		// Log custom classes (new format)
		if len(config.Classes.CustomClasses) > 0 {
			for i, customClass := range config.Classes.CustomClasses {
				logger.Info("Custom class configuration",
					"classIndex", i+1,
					"className", customClass.Name,
					"itemsCount", len(customClass.Items),
					"boost", customClass.Boost)
				for j, item := range customClass.Items {
					logger.Debug("Custom class item detail",
						"classIndex", i+1,
						"className", customClass.Name,
						"itemIndex", j+1,
						"item", item,
						"itemLength", len(item))
				}
			}
		}

		// Log legacy custom class items
		if len(config.Classes.CustomClassItems) > 0 {
			logger.Info("Legacy custom class items received",
				"itemsCount", len(config.Classes.CustomClassItems),
				"boost", config.Classes.Boost)
			for i, item := range config.Classes.CustomClassItems {
				logger.Debug("Legacy custom class item detail",
					"index", i+1,
					"item", item,
					"itemLength", len(item))
			}
		}
	} else {
		logger.Debug("No classes configuration provided")
	}

	// Debug: Log the exact format string received
	logger.Debug("Exact audio format received", "format", config.AudioFormat.Format)

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

	// Create speech contexts using the new advanced configuration
	var speechContexts []*speechpb.SpeechContext
	speechContexts = createAdvancedSpeechContexts(config.CustomWords, config.PhraseSets, config.Classes)
	if speechContexts != nil && len(speechContexts) > 0 {
		logger.Info("Using advanced SpeechContexts for enhanced recognition", "totalContexts", len(speechContexts))
	}
	
	// Store initial speech contexts and keywords for dynamic updates
	keywordsMu.Lock()
	currentSpeechContexts = make([]*speechpb.SpeechContext, len(speechContexts))
	copy(currentSpeechContexts, speechContexts)
	dynamicKeywords = make([]string, len(config.CustomWords))
	copy(dynamicKeywords, config.CustomWords)
	keywordsMu.Unlock()

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
	recognitionConfig := &speechpb.RecognitionConfig{
		Encoding:                 encoding,
		SampleRateHertz:          int32(config.AudioFormat.SampleRate),
		LanguageCode:             primaryLanguage,
		AlternativeLanguageCodes: alternativeLanguages,
	}

	// Add speech contexts if available
	if speechContexts != nil && len(speechContexts) > 0 {
		recognitionConfig.SpeechContexts = speechContexts
		logger.Info("Applied SpeechContexts to recognition configuration",
			"contextsCount", len(speechContexts),
			"encoding", encoding,
			"sampleRate", config.AudioFormat.SampleRate,
			"primaryLanguage", primaryLanguage)

		// Log detailed context application
		var totalPhrases int
		var totalBoostSum float32
		for i, context := range speechContexts {
			totalPhrases += len(context.Phrases)
			totalBoostSum += context.Boost
			logger.Debug("Applied SpeechContext to recognition config",
				"contextIndex", i+1,
				"phrasesInContext", len(context.Phrases),
				"contextBoost", context.Boost)
		}
		averageBoost := totalBoostSum / float32(len(speechContexts))
		logger.Info("SpeechContexts application summary",
			"totalContexts", len(speechContexts),
			"totalPhrases", totalPhrases,
			"averageBoost", averageBoost)
	} else {
		logger.Info("No SpeechContexts to apply to recognition configuration",
			"encoding", encoding,
			"sampleRate", config.AudioFormat.SampleRate,
			"primaryLanguage", primaryLanguage)
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
	
	// Dynamic keyword management (using global variables)
	
	// Function to create or recreate the stream with optional updated speech contexts
	createStream := func(updatedContexts []*speechpb.SpeechContext) error {
		streamMu.Lock()
		defer streamMu.Unlock()
		
		// Close existing stream if it exists
		if stream != nil {
			stream.CloseSend()
			stream = nil
		}
		
		// Check if context is still valid before creating new stream
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled, cannot create new stream: %v", ctx.Err())
		}
		
		// Create a new bidirectional streaming RPC
		newStream, err := client.StreamingRecognize(ctx)
		if err != nil {
			return fmt.Errorf("failed to create streaming client: %v", err)
		}
		
		// Create updated recognition config with new contexts if provided
		currentRecognitionConfig := &speechpb.RecognitionConfig{
			Encoding:                 encoding,
			SampleRateHertz:          int32(config.AudioFormat.SampleRate),
			LanguageCode:             primaryLanguage,
			AlternativeLanguageCodes: alternativeLanguages,
		}
		
		// Use updated contexts if provided, otherwise use original speech contexts
		var contextsToUse []*speechpb.SpeechContext
		if updatedContexts != nil {
			contextsToUse = updatedContexts
			logger.Info("Using updated SpeechContexts for stream recreation",
				"contextsCount", len(updatedContexts))
		} else {
			contextsToUse = speechContexts
			logger.Debug("Using original SpeechContexts for stream recreation",
				"contextsCount", len(speechContexts))
		}
		
		if len(contextsToUse) > 0 {
			currentRecognitionConfig.SpeechContexts = contextsToUse
		}
		
		// Create updated request template
		currentReqTemplate := speechpb.StreamingRecognizeRequest{
			StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
				StreamingConfig: &speechpb.StreamingRecognitionConfig{
					Config:         currentRecognitionConfig,
					InterimResults: true,
				},
			},
		}
		
		// Send the initial configuration message
		if err := newStream.Send(&currentReqTemplate); err != nil {
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
	if err := createStream(nil); err != nil {
		logger.Error("Failed to create initial stream", "error", err)
		return
	}

	var fullTranscription strings.Builder
	var currentSummary string
	var summaryMu sync.Mutex // Protect currentSummary from race conditions
	customWords := config.CustomWords // Store custom words for use in summary generation

	// Default prompt for summarization
	defaultSummaryPrompt := `You are tasked with creating and maintaining a summary of a live conversation transcript. Follow these guidelines:

1. **Language**: Write the summary in the same language as the majority of the transcript
2. **Iterative approach**: Keep the initial summary as much as possible and only make changes if there are inconsistencies, nonsensical parts, or incoherent content
3. **Completion**: Simply complete or extend the summary with new information from the transcript
4. **Accuracy**: Do not invent or add information that is not present in the transcript
5. **Important quotes**: When something is particularly important, include a direct quote from the transcript
6. **Format**: Use markdown formatting for better readability. Put emphasis (bold and italic) on important concept, and use > for quotes.

If this is an update to an existing summary, maintain the structure and content of the previous summary unless corrections are needed.`

	// Get summarization prompt from config, or use default
	summaryPrompt := config.SummaryPrompt
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
				if recreateErr := createStream(nil); recreateErr != nil {
					// Check if the error is due to connection closing
					if ctx.Err() != nil {
						logger.Info("Context cancelled during stream recreation, stopping receive loop")
						return
					}
					logger.Error("Failed to recreate stream", "error", recreateErr)
					return
				}
				// After recreation, continue to get new stream reference
				continue
			}
			if err != nil {
				logger.Error("Error receiving from Speech-to-Text", "error", err)
				// Check if this is a context cancellation error
				if ctx.Err() != nil {
					logger.Info("Context cancelled, stopping receive loop")
					return
				}
				// Try to recreate stream on error
				if recreateErr := createStream(nil); recreateErr != nil {
					// Check if the error is due to connection closing
					if ctx.Err() != nil {
						logger.Info("Context cancelled during stream recreation, stopping receive loop")
						return
					}
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
								summary, err := generateSummary(ctx, projectID, location, fullTranscript, previousSummary, summaryPrompt, customWords)
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
					if err := createStream(nil); err != nil {
						// Check if the error is due to connection closing
						if ctx.Err() != nil {
							logger.Info("Context cancelled during stream recreation, stopping duration monitoring")
							return
						}
						logger.Error("Failed to recreate stream due to duration limit", "error", err)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Channel to coordinate final summary completion before closing
	finalSummaryDone := make(chan struct{})
	var finalSummaryInProgress int32 // atomic counter

	// Main loop to read from client and send audio to Speech-to-Text
	audioChunkCount := 0
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Error("Unexpected WebSocket error", "error", err)
			} else {
				logger.Info("WebSocket connection closed by client")
				
				// If final summary is in progress, wait for it to complete
				if atomic.LoadInt32(&finalSummaryInProgress) > 0 {
					logger.Info("Waiting for final summary to complete before closing connection")
					select {
					case <-finalSummaryDone:
						logger.Info("Final summary completed, proceeding with connection closure")
					case <-time.After(35 * time.Second): // Slightly longer than the summary timeout
						logger.Warn("Timeout waiting for final summary, proceeding with connection closure")
					}
				}
			}
			// Cancel context when WebSocket closes to stop all related goroutines
			cancel()
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
					if recreateErr := createStream(nil); recreateErr != nil {
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
			
			// Parse the message to determine its type
			var baseMessage struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(message, &baseMessage); err != nil {
				logger.Warn("Failed to parse message type", "error", err, "message", string(message))
				continue
			}
			
			switch baseMessage.Type {
			case "config":
				// Check if it's a new config message (for system audio mode)
				var newConfig ConfigMessage
				if err := json.Unmarshal(message, &newConfig); err == nil {
					logger.Info("Received new config message", "config", newConfig)
				}
			case "end_prompt":
				// Handle end prompt message (final summary generation when stopping)
				logger.Info("End prompt message received",
					"rawMessage", string(message))

				var endPromptMsg EndPromptMessage
				if err := json.Unmarshal(message, &endPromptMsg); err != nil {
					logger.Error("Failed to parse end prompt message",
						"error", err,
						"rawMessage", string(message),
						"messageLength", len(message))
					continue
				}
				
				logger.Info("End prompt processed successfully",
					"endPrompt", endPromptMsg.EndPrompt,
					"clientTimestamp", endPromptMsg.Timestamp,
					"serverTimestamp", time.Now(),
					"timeDelta", time.Since(endPromptMsg.Timestamp))

				// Generate final summary with end prompt asynchronously
				if projectID != "" && location != "" {
					// Mark that final summary generation is starting
					atomic.AddInt32(&finalSummaryInProgress, 1)
					go func() {
						defer func() {
							// Mark final summary as complete and signal completion
							atomic.AddInt32(&finalSummaryInProgress, -1)
							select {
							case finalSummaryDone <- struct{}{}:
							default: // Non-blocking send
							}
						}()
						
						// Create a new context with timeout for the end prompt generation
						// This prevents cancellation when WebSocket closes
						endPromptCtx, endPromptCancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer endPromptCancel()

						fullTranscript := strings.TrimSpace(fullTranscription.String())
						if fullTranscript == "" {
							logger.Warn("No transcript available for end prompt summary")
							return
						}

						// Safely read current summary
						summaryMu.Lock()
						previousSummary := currentSummary
						summaryMu.Unlock()

						// Combine original summary prompt with end prompt
						combinedPrompt := summaryPrompt + "\n\n" + endPromptMsg.EndPrompt

						logger.Info("Generating final summary with end prompt",
							"transcriptLength", len(fullTranscript),
							"previousSummaryLength", len(previousSummary),
							"combinedPromptLength", len(combinedPrompt))

						summary, err := generateSummary(endPromptCtx, projectID, location, fullTranscript, previousSummary, combinedPrompt, customWords)
						if err != nil {
							logger.Error("Error generating final summary with end prompt", "error", err)
							return
						}
						if summary != "" {
							// Safely update current summary
							summaryMu.Lock()
							currentSummary = summary
							summaryMu.Unlock()

							logger.Info("Final summary with end prompt generated", "summaryLength", len(summary))
							summaryResponse := SummaryResponse{
								Type:      "summary",
								Text:      summary,
								Timestamp: time.Now(),
							}
							summaryData, err := json.Marshal(summaryResponse)
							if err != nil {
								logger.Error("Failed to marshal final summary response", "error", err)
								return
							}
							
							// Check if WebSocket is still open before sending
							mu.Lock()
							defer mu.Unlock()
							
							if conn != nil {
								// Set a write deadline to prevent blocking on a dead connection
								conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
								
								logger.Info("Sending final summary to client",
									"summaryLength", len(summary),
									"connectionState", "open")
								
								if err := conn.WriteMessage(websocket.TextMessage, summaryData); err != nil {
									logger.Warn("Failed to send final summary to client", 
										"error", err,
										"errorType", fmt.Sprintf("%T", err))
								} else {
									logger.Info("Final summary sent to client successfully",
										"summaryLength", len(summary))
								}
								
								// Clear the write deadline
								conn.SetWriteDeadline(time.Time{})
							} else {
								logger.Warn("WebSocket connection is nil, final summary generated but not sent",
									"summaryLength", len(summary))
							}
						}
					}()
				} else {
					logger.Warn("GCP configuration not available for end prompt summary generation")
				}
			case "keywords":
				// Handle keywords message (dynamic keyword updates during recording)
				logger.Info("Dynamic keywords update received",
					"rawMessage", string(message))

				var keywordsMsg KeywordsMessage
				if err := json.Unmarshal(message, &keywordsMsg); err != nil {
					logger.Error("Failed to parse keywords message",
						"error", err,
						"rawMessage", string(message),
						"messageLength", len(message))
					continue
				}
				
				logger.Info("Dynamic keywords processed successfully",
					"words", keywordsMsg.Words,
					"wordCount", len(keywordsMsg.Words),
					"clientTimestamp", keywordsMsg.Timestamp,
					"serverTimestamp", time.Now(),
					"timeDelta", time.Since(keywordsMsg.Timestamp))
				
				// Log each individual keyword for detailed tracking
				for i, word := range keywordsMsg.Words {
					trimmedWord := strings.TrimSpace(word)
					logger.Info("Dynamic keyword detail",
						"index", i+1,
						"originalWord", word,
						"trimmedWord", trimmedWord,
						"wordLength", len(word),
						"trimmedLength", len(trimmedWord),
						"isEmpty", trimmedWord == "")
				}

				// Update dynamic keywords and recreate stream with new SpeechContexts
				keywordsMu.Lock()
				// Add new keywords to existing dynamic keywords (avoiding duplicates)
				existingKeywords := make(map[string]bool)
				for _, existing := range dynamicKeywords {
					existingKeywords[strings.ToLower(strings.TrimSpace(existing))] = true
				}
				
				var newKeywordsToAdd []string
				for _, newKeyword := range keywordsMsg.Words {
					trimmed := strings.TrimSpace(newKeyword)
					if trimmed != "" && !existingKeywords[strings.ToLower(trimmed)] {
						newKeywordsToAdd = append(newKeywordsToAdd, trimmed)
						dynamicKeywords = append(dynamicKeywords, trimmed)
						existingKeywords[strings.ToLower(trimmed)] = true
					}
				}
				
				logger.Info("Dynamic keywords update processed",
					"newKeywordsAdded", len(newKeywordsToAdd),
					"totalDynamicKeywords", len(dynamicKeywords),
					"newKeywords", newKeywordsToAdd,
					"allDynamicKeywords", dynamicKeywords)
				
				// Create updated speech contexts combining original + dynamic keywords
				updatedContexts := createDynamicSpeechContexts(currentSpeechContexts, dynamicKeywords)
				keywordsMu.Unlock()
				
				// Recreate stream with updated contexts if we have new keywords
				if len(newKeywordsToAdd) > 0 {
					logger.Info("Recreating Speech-to-Text stream with dynamic keywords",
						"newKeywordsCount", len(newKeywordsToAdd),
						"totalDynamicKeywords", len(dynamicKeywords),
						"updatedContextsCount", len(updatedContexts))
					
					if err := createStream(updatedContexts); err != nil {
						logger.Error("Failed to recreate stream with dynamic keywords",
							"error", err,
							"newKeywords", newKeywordsToAdd)
					} else {
						logger.Info("Stream successfully recreated with dynamic keywords",
							"appliedKeywords", newKeywordsToAdd,
							"totalKeywords", len(dynamicKeywords))
					}
				} else {
					logger.Info("No new keywords to apply - all keywords already exist",
						"duplicateKeywords", keywordsMsg.Words,
						"existingDynamicKeywords", dynamicKeywords)
				}
			default:
				logger.Debug("Received unknown message type", "type", baseMessage.Type, "message", string(message))
			}
		}
	}

	// Close the Speech-to-Text stream when the WebSocket connection closes
	streamMu.Lock()
	if stream != nil {
		logger.Info("Closing Speech-to-Text stream due to WebSocket closure")
		stream.CloseSend()
	}
	streamMu.Unlock()
	
	// Ensure context is cancelled to stop all related goroutines
	cancel()
	logger.Info("WebSocket connection and Speech-to-Text stream closed")
}

// serveDefaultPrompt serves the default summary prompt as JSON
func serveDefaultPrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defaultSummaryPrompt := `You are tasked with creating and maintaining a summary of a live conversation transcript. Follow these guidelines:

1. **Language**: Write the summary in the same language as the majority of the transcript
2. **Iterative approach**: Keep the initial summary as much as possible and only make changes if there are inconsistencies, nonsensical parts, or incoherent content
3. **Completion**: Simply complete or extend the summary with new information from the transcript
4. **Accuracy**: Do not invent or add information that is not present in the transcript
5. **Important quotes**: When something is particularly important, include a direct quote from the transcript
6. **Format**: Use markdown formatting for better readability. Put emphasis (bold and italic) on important concept, and use > for quotes.

If this is an update to an existing summary, maintain the structure and content of the previous summary unless corrections are needed.`

	defaultEndPrompt := `**IMPORTANT**: Keep the existing summary language and maintain the structure above.

Now add a **conclusion** to finalize this conversation summary. Include:

## Conclusion
- **Key Points**: Summarize the main takeaways from the conversation
- **Important Decisions**: Highlight any decisions or agreements made
- **Action Items**: List specific next steps or tasks identified
- **Follow-up**: Note any planned future discussions or meetings

Ensure the conclusion flows naturally from the existing summary and provides clear closure to the conversation.`

	response := map[string]string{
		"defaultPrompt":    defaultSummaryPrompt,
		"defaultEndPrompt": defaultEndPrompt,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Error("Failed to encode default prompt response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// serveStaticFiles serves static HTML and JS files
func serveStaticFiles(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		http.ServeFile(w, r, "live_transcription_ui.html")
	case "/live_audio_recorder.js":
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "live_audio_recorder.js")
	case "/audio-processor.js":
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "audio-processor.js")
	case "/favicon.ico":
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, "favicon.png")
	case "/favicon.png":
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, "favicon.png")
	default:
		http.NotFound(w, r)
	}
}

func main() {
	// Initialize logging
	initLogger()

	// Set up routes
	http.HandleFunc("/ws", handleWebSocket)
	http.HandleFunc("/api/default-prompt", serveDefaultPrompt)
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
