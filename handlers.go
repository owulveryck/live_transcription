package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed ui
var uiFiles embed.FS

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

If this is an update to an existing summary, maintain the structure and content of the previous summary unless corrections are needed.
**IMPORTANT**: Keep the existing summary language and maintain the structure of the previous summary.`

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

// serveStaticFiles serves static files from embedded filesystem
func serveStaticFiles(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Handle root path
	if path == "/" {
		// Parse and execute HTML template with WebSocket configuration
		tmplContent, err := uiFiles.ReadFile("ui/live_transcription_ui.html")
		if err != nil {
			logger.Error("Failed to read HTML template from embedded FS", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		tmpl, err := template.New("index").Parse(string(tmplContent))
		if err != nil {
			logger.Error("Failed to parse HTML template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Get WebSocket host from environment variable or default to empty (client auto-detection)
		wsHost := os.Getenv("WEBSOCKET_HOST")
		if wsHost == "" {
			// Let client auto-detect from current page host
			wsHost = ""
		}

		data := TemplateData{
			WebSocketHost: wsHost,
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			logger.Error("Failed to execute HTML template", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	// Handle UI files
	if strings.HasPrefix(path, "/ui/") {
		// Remove leading slash to match embedded path structure
		filePath := strings.TrimPrefix(path, "/")

		// Read file from embedded filesystem
		content, err := uiFiles.ReadFile(filePath)
		if err != nil {
			logger.Error("Failed to read file from embedded FS", "path", filePath, "error", err)
			http.NotFound(w, r)
			return
		}

		// Set appropriate content type based on file extension
		contentType := getContentType(filePath)
		w.Header().Set("Content-Type", contentType)

		// Write file content
		w.Write(content)
		return
	}

	// Handle direct file requests (for relative paths from served JS files)
	// Try to serve files from ui directory first
	possiblePaths := []string{
		"ui" + path,     // /css/styles.css -> ui/css/styles.css
		"ui/js" + path,  // /audio-processor.js -> ui/js/audio-processor.js
		"ui/css" + path, // /styles.css -> ui/css/styles.css
	}

	for _, tryPath := range possiblePaths {
		content, err := uiFiles.ReadFile(tryPath)
		if err == nil {
			// File found, serve it
			contentType := getContentType(tryPath)
			w.Header().Set("Content-Type", contentType)
			w.Write(content)
			return
		}
	}

	// Handle favicon requests
	if path == "/favicon.ico" || path == "/favicon.png" {
		content, err := uiFiles.ReadFile("ui/favicon.png")
		if err != nil {
			logger.Error("Failed to read favicon from embedded FS", "error", err)
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Write(content)
		return
	}

	http.NotFound(w, r)
}

// getContentType returns the appropriate content type based on file extension
func getContentType(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json"
	case ".ico":
		return "image/x-icon"
	default:
		return "text/plain"
	}
}

// getPresetDirectory returns the preset directory path from environment or default
func getPresetDirectory() string {
	dir := os.Getenv("PRESET_DIRECTORY")
	if dir == "" {
		dir = "./presets"
	}
	return dir
}

// parsePresetFile parses a preset file and returns a Preset struct
func parsePresetFile(content string) (*Preset, error) {
	preset := &Preset{}
	lines := strings.Split(content, "\n")

	var currentSection string
	var summaryLines, conclusionLines []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Title: ") {
			preset.Title = strings.TrimPrefix(line, "Title: ")
		} else if strings.HasPrefix(line, "Summary: ") {
			currentSection = "summary"
			summaryLines = append(summaryLines, strings.TrimPrefix(line, "Summary: "))
		} else if strings.HasPrefix(line, "Conclusion: ") {
			currentSection = "conclusion"
			conclusionLines = append(conclusionLines, strings.TrimPrefix(line, "Conclusion: "))
		} else if line != "" {
			switch currentSection {
			case "summary":
				summaryLines = append(summaryLines, line)
			case "conclusion":
				conclusionLines = append(conclusionLines, line)
			}
		}
	}

	preset.Summary = strings.Join(summaryLines, "\n")
	preset.Conclusion = strings.Join(conclusionLines, "\n")

	return preset, nil
}

// servePresets serves the list of available presets
func servePresets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	presetDir := getPresetDirectory()
	presets := make(map[string]string)

	// Check if directory exists
	if _, err := os.Stat(presetDir); os.IsNotExist(err) {
		logger.Warn("Preset directory does not exist", "directory", presetDir)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(presets)
		return
	}

	// Read directory contents
	files, err := os.ReadDir(presetDir)
	if err != nil {
		logger.Error("Failed to read preset directory", "directory", presetDir, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Process each .txt file
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".txt") {
			continue
		}

		filePath := filepath.Join(presetDir, file.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			logger.Error("Failed to read preset file", "file", filePath, "error", err)
			continue
		}

		preset, err := parsePresetFile(string(content))
		if err != nil {
			logger.Error("Failed to parse preset file", "file", filePath, "error", err)
			continue
		}

		presetName := strings.TrimSuffix(file.Name(), ".txt")
		presets[presetName] = preset.Title
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(presets); err != nil {
		logger.Error("Failed to encode presets response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// servePreset serves a specific preset's content
func servePreset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract preset name from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/presets/")
	if path == "" {
		http.Error(w, "Preset name required", http.StatusBadRequest)
		return
	}

	presetDir := getPresetDirectory()
	filePath := filepath.Join(presetDir, path+".txt")

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Preset not found", http.StatusNotFound)
		return
	}

	// Read and parse preset file
	content, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("Failed to read preset file", "file", filePath, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	preset, err := parsePresetFile(string(content))
	if err != nil {
		logger.Error("Failed to parse preset file", "file", filePath, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(preset); err != nil {
		logger.Error("Failed to encode preset response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
