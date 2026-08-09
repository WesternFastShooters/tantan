package recommendation_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/recommendation"
)

type rankingGoldenItem struct {
	EntryID string                        `json:"entryId"`
	Score   recommendation.ScoreBreakdown `json:"score"`
}

func TestRankIsDeterministicAndEnforcesDiversity(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	candidates := make([]recommendation.Candidate, 0, 80)
	for index := 0; index < 80; index++ {
		candidates = append(candidates, recommendation.Candidate{
			EntryID:        fmt.Sprintf("entry_%03d", index),
			SourceID:       fmt.Sprintf("source_%02d", index%12),
			PrimaryTopicID: fmt.Sprintf("topic_%02d", index%10),
			PublishedAt:    now.Add(-time.Duration(index) * time.Minute),
			Quality:        float64(15 - index%4),
			TopicAffinity:  float64(index % 21),
			SourceAffinity: float64(index % 16),
		})
	}
	first := recommendation.Rank(now, candidates, 50)
	second := recommendation.Rank(now, append([]recommendation.Candidate(nil), candidates...), 50)
	if !reflect.DeepEqual(first, second) || len(first) != 50 {
		t.Fatalf("rank is unstable or incomplete: first=%d second=%d", len(first), len(second))
	}
	sources := map[string]int{}
	topics := map[string]int{}
	for index, item := range first[:20] {
		sources[item.SourceID]++
		topics[item.PrimaryTopicID]++
		if sources[item.SourceID] > 3 || topics[item.PrimaryTopicID] > 5 {
			t.Fatalf("diversity violated at %d: sources=%v topics=%v", index, sources, topics)
		}
		if index >= 2 && first[index-1].SourceID == item.SourceID && first[index-2].SourceID == item.SourceID {
			t.Fatalf("three consecutive cards from %s", item.SourceID)
		}
	}
	for index := 2; index < len(first); index++ {
		if first[index-1].SourceID == first[index].SourceID && first[index-2].SourceID == first[index].SourceID {
			t.Fatalf("three consecutive cards from %s", first[index].SourceID)
		}
	}
}

func TestScoreUsesApprovedClampsAndStableTieBreakers(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	candidate := recommendation.Candidate{
		EntryID:        "entry_b",
		SourceID:       "source_1",
		PrimaryTopicID: "topic_1",
		PublishedAt:    now.Add(-84 * time.Hour),
		Quality:        99,
		TopicAffinity:  99,
		SourceAffinity: 99,
		FilterMatch:    99,
	}
	scored := recommendation.Score(now, candidate)
	if scored.Recency != 20 || scored.TopicAffinity != 20 || scored.SourceAffinity != 15 || scored.Quality != 15 || scored.FilterMatch != 30 || scored.Total != 100 {
		t.Fatalf("score=%#v", scored)
	}
	tied := candidate
	tied.EntryID = "entry_a"
	ranked := recommendation.Rank(now, []recommendation.Candidate{candidate, tied}, 2)
	if len(ranked) != 2 || ranked[0].EntryID != "entry_a" || ranked[1].EntryID != "entry_b" {
		t.Fatalf("tie order=%#v", ranked)
	}
}

func TestRankingGoldenOrderAndScoreReasons(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	candidates := []recommendation.Candidate{
		{EntryID: "entry_a", SourceID: "source_1", PrimaryTopicID: "topic_1", PublishedAt: now, Quality: 5, TopicAffinity: 10, SourceAffinity: 5},
		{EntryID: "entry_b", SourceID: "source_1", PrimaryTopicID: "topic_1", PublishedAt: now.Add(-84 * time.Hour), Quality: 15, TopicAffinity: 20, SourceAffinity: 15},
		{EntryID: "entry_c", SourceID: "source_2", PrimaryTopicID: "topic_2", PublishedAt: now.Add(-168 * time.Hour), Quality: 15, TopicAffinity: 20, SourceAffinity: 15},
		{EntryID: "entry_d", SourceID: "source_1", PrimaryTopicID: "topic_3", PublishedAt: now, Quality: 15, TopicAffinity: 20, SourceAffinity: 15, FilterMatch: 30},
	}
	ranked := recommendation.Rank(now, candidates, len(candidates))
	actual := make([]rankingGoldenItem, 0, len(ranked))
	for _, item := range ranked {
		actual = append(actual, rankingGoldenItem{EntryID: item.EntryID, Score: item.Score})
	}
	contents, err := os.ReadFile("testdata/ranking_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected []rankingGoldenItem
	if err := json.Unmarshal(contents, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("ranking golden changed:\nactual=%#v\nexpected=%#v", actual, expected)
	}
}
