package main

import (
	"time"
)

// AudioFormat represents the audio format configuration from the client
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
	CustomClassItems []string `json:"customClassItems,omitempty"`
	Boost            float32  `json:"boost,omitempty"`
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

// TemplateData holds data for serving the HTML template
type TemplateData struct {
	WebSocketHost string
}