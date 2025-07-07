package main

import (
	"strings"
	"sync"

	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

// Global variables for dynamic keyword management
var keywordsMu sync.Mutex
var currentSpeechContexts []*speechpb.SpeechContext
var dynamicKeywords []string

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