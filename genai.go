package main

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

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