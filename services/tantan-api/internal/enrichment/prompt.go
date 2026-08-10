package enrichment

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"tantan.local/tantan-api/internal/ai"
	"tantan.local/tantan-api/internal/topic"
)

type entryInput struct {
	Title       string
	Description string
	Content     string
	Language    string
}

func enrichmentPrompt(input entryInput, language, promptVersion string) ai.GenerationRequest {
	payload, _ := json.Marshal(map[string]string{
		"title":          truncate(input.Title, 500),
		"description":    truncate(input.Description, 4_000),
		"content":        truncate(input.Content, 100_000),
		"sourceLanguage": truncate(input.Language, 16),
		"targetLanguage": language,
	})
	return ai.GenerationRequest{
		SchemaName: ai.EnrichmentSchemaName,
		SystemPrompt: fmt.Sprintf(
			"Tantan %s. Return one JSON object only. It must exactly follow AIEnrichmentV1 version 1 with keys version, detectedLanguage, titleZh, contentZh, summaryZh, keyPoints. titleZh and contentZh must always be non-empty Simplified Chinese strings; when the input is already Chinese, copy and normalize it instead of returning null. Preserve the complete meaning of the title and article body. No Markdown, HTML, URLs, tools, or extra keys.",
			promptVersion,
		),
		UserPrompt: string(payload),
	}
}

func topicPrompt(input entryInput, topics []topic.Item, promptVersion string) ai.GenerationRequest {
	allowed := make([]map[string]string, 0, len(topics))
	for _, item := range topics {
		if item.Kind == "virtual" || item.Hidden {
			continue
		}
		allowed = append(allowed, map[string]string{"topicId": item.ID, "name": item.Name})
	}
	payload, _ := json.Marshal(map[string]any{
		"title":         truncate(input.Title, 500),
		"description":   truncate(input.Description, 4_000),
		"content":       truncate(input.Content, 100_000),
		"allowedTopics": allowed,
	})
	return ai.GenerationRequest{
		SchemaName: ai.TopicSchemaName,
		SystemPrompt: fmt.Sprintf(
			"Tantan %s. Choose 1 to 5 topicId values only from allowedTopics. Return one JSON object exactly matching TopicClassificationV1 version 1. No extra keys, Markdown, HTML, URLs, or tools.",
			promptVersion,
		),
		UserPrompt: string(payload),
	}
}

func repairPrompt(original ai.GenerationRequest, invalid []byte) ai.GenerationRequest {
	payload, _ := json.Marshal(map[string]string{
		"invalidOutput": truncate(string(invalid), 200_000),
		"schema":        original.SchemaName,
	})
	return ai.GenerationRequest{
		SchemaName:   original.SchemaName,
		SystemPrompt: "Repair the supplied output once. Return only a valid JSON object matching the requested schema exactly. Do not add Markdown or explanation.",
		UserPrompt:   string(payload),
		Repair:       true,
	}
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
