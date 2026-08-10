package enrichment

import (
	"strings"
	"testing"
)

func TestEnrichmentPromptRequiresDisplayableChineseContent(t *testing.T) {
	request := enrichmentPrompt(entryInput{
		Title:    "An English title",
		Content:  "An English article body.",
		Language: "en",
	}, "zh-CN", "prompt-v1")

	for _, requirement := range []string{
		"must always be non-empty Simplified Chinese strings",
		"already Chinese",
		"instead of returning null",
		"complete meaning",
	} {
		if !strings.Contains(request.SystemPrompt, requirement) {
			t.Fatalf("prompt missing requirement %q: %s", requirement, request.SystemPrompt)
		}
	}
}
