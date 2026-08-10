package filter

import (
	"reflect"
	"testing"

	"tantan.local/tantan-api/internal/recommendation"
)

func TestAlignSpecWithPromptDropsUnrequestedModelConstraints(t *testing.T) {
	spec := recommendation.FilterSpecV1{
		Version:        1,
		WindowDays:     10,
		IncludeTopics:  []string{},
		ExcludeTopics:  []string{},
		IncludeSources: []string{},
		ExcludeSources: []string{},
		IncludeTerms:   []string{},
		NegativeTerms:  []string{},
		Languages:      []string{"zh-CN", "en"},
		ContentStyles:  []string{"article", "post"},
		Weights: recommendation.Weights{
			Freshness: 1, TopicMatch: 1, SourceAffinity: 1, Quality: 1, Diversity: 1,
		},
	}
	aligned, canonical, err := alignSpecWithPrompt("最近一周多推 SpaceX、星链和航天内容，不要政治新闻", spec)
	if err != nil {
		t.Fatal(err)
	}
	if aligned.WindowDays != 7 || len(aligned.Languages) != 0 || len(aligned.ContentStyles) != 0 || len(canonical) == 0 {
		t.Fatalf("aligned=%#v canonical=%s", aligned, canonical)
	}
}

func TestAlignSpecWithPromptPreservesExplicitLanguageAndStyle(t *testing.T) {
	spec := recommendation.FilterSpecV1{
		Version:        1,
		WindowDays:     7,
		IncludeTopics:  []string{},
		ExcludeTopics:  []string{},
		IncludeSources: []string{},
		ExcludeSources: []string{},
		IncludeTerms:   []string{},
		NegativeTerms:  []string{},
		Languages:      []string{"en"},
		ContentStyles:  []string{"article"},
		Weights: recommendation.Weights{
			Freshness: 1, TopicMatch: 1, SourceAffinity: 1, Quality: 1, Diversity: 1,
		},
	}
	aligned, _, err := alignSpecWithPrompt("只看英文文章", spec)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aligned.Languages, []string{"en"}) || !reflect.DeepEqual(aligned.ContentStyles, []string{"article"}) {
		t.Fatalf("aligned=%#v", aligned)
	}
}
