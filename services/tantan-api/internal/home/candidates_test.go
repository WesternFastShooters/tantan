package home

import (
	"testing"
	"time"

	"tantan.local/tantan-api/internal/recommendation"
)

func TestApplyFilterTreatsPositiveSignalsAsAlternatives(t *testing.T) {
	record := candidateRecord{
		Candidate: recommendation.Candidate{
			EntryID:     "entry_spacex",
			SourceID:    "feed_elon",
			PublishedAt: time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		},
		Title:    "Falcon 9 launches Starlink satellites",
		Body:     "SpaceX mission from California",
		Kind:     "article",
		TopicIDs: []string{"topic_space"},
	}
	spec := &recommendation.FilterSpecV1{
		IncludeTopics:  []string{"topic_space"},
		IncludeSources: []string{"another_source"},
		IncludeTerms:   []string{"unmatched-term"},
		Weights: recommendation.Weights{
			Freshness: 1, TopicMatch: 1, SourceAffinity: 1, Quality: 1, Diversity: 1,
		},
	}
	selected, ok := applyFilter(record, spec)
	if !ok || selected.EntryID != record.Candidate.EntryID || selected.FilterMatch != 10 {
		t.Fatalf("selected=%#v ok=%v", selected, ok)
	}

	spec.IncludeTopics = []string{"topic_other"}
	if _, ok := applyFilter(record, spec); ok {
		t.Fatal("candidate without any positive match was selected")
	}
}

func TestApplyFilterInfersMissingUpstreamLanguage(t *testing.T) {
	base := recommendation.Candidate{EntryID: "entry", SourceID: "feed", PublishedAt: time.Now()}
	english := candidateRecord{Candidate: base, Title: "SpaceX launches Starlink", Kind: "article"}
	chinese := candidateRecord{Candidate: base, Title: "星链卫星发射", Kind: "article"}
	englishSpec := &recommendation.FilterSpecV1{Languages: []string{"en"}}
	chineseSpec := &recommendation.FilterSpecV1{Languages: []string{"zh-CN"}}
	if _, ok := applyFilter(english, englishSpec); !ok {
		t.Fatal("missing language metadata did not infer English")
	}
	if _, ok := applyFilter(chinese, chineseSpec); !ok {
		t.Fatal("missing language metadata did not infer Chinese")
	}
}
