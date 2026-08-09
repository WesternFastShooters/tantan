package recommendation

import (
	"sort"
	"time"
)

type Weights struct {
	Freshness      float64 `json:"freshness"`
	TopicMatch     float64 `json:"topicMatch"`
	SourceAffinity float64 `json:"sourceAffinity"`
	Quality        float64 `json:"quality"`
	Diversity      float64 `json:"diversity"`
}

type Candidate struct {
	EntryID        string
	SourceID       string
	PrimaryTopicID string
	PublishedAt    time.Time
	Quality        float64
	TopicAffinity  float64
	SourceAffinity float64
	FilterMatch    float64
	FilterWeights  *Weights
}

type ScoredCandidate struct {
	Candidate
	Score ScoreBreakdown
}

type ScoreBreakdown struct {
	Recency        float64 `json:"recency"`
	TopicAffinity  float64 `json:"topicAffinity"`
	SourceAffinity float64 `json:"sourceAffinity"`
	Quality        float64 `json:"quality"`
	FilterMatch    float64 `json:"filterMatch"`
	Total          float64 `json:"total"`
}

func Score(now time.Time, candidate Candidate) ScoreBreakdown {
	ageHours := now.Sub(candidate.PublishedAt).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	weights := Weights{Freshness: 1, TopicMatch: 1, SourceAffinity: 1, Quality: 1, Diversity: 1}
	if candidate.FilterWeights != nil {
		weights = *candidate.FilterWeights
	}
	result := ScoreBreakdown{
		Recency:        clamp(40*max(0, 1-ageHours/168), 0, 40) * clamp(weights.Freshness, 0, 2),
		TopicAffinity:  clamp(candidate.TopicAffinity, 0, 20) * clamp(weights.TopicMatch, 0, 2),
		SourceAffinity: clamp(candidate.SourceAffinity, 0, 15) * clamp(weights.SourceAffinity, 0, 2),
		Quality:        clamp(candidate.Quality, 0, 15) * clamp(weights.Quality, 0, 2),
		FilterMatch:    clamp(candidate.FilterMatch, 0, 30),
	}
	result.Total = result.Recency + result.TopicAffinity + result.SourceAffinity + result.Quality + result.FilterMatch
	return result
}

func Rank(now time.Time, candidates []Candidate, limit int) []ScoredCandidate {
	return rank(now, candidates, limit, nil, true)
}

// RankAfterSources ranks append candidates without losing the source continuity
// of the already-persisted queue. Only the last two sources affect the result.
func RankAfterSources(now time.Time, candidates []Candidate, limit int, previousSources []string) []ScoredCandidate {
	if len(previousSources) > 2 {
		previousSources = previousSources[len(previousSources)-2:]
	}
	return rank(now, candidates, limit, previousSources, false)
}

func rank(now time.Time, candidates []Candidate, limit int, previousSources []string, enforceFirstTwentyCounts bool) []ScoredCandidate {
	if limit <= 0 || len(candidates) == 0 {
		return []ScoredCandidate{}
	}
	remaining := make([]ScoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		remaining = append(remaining, ScoredCandidate{Candidate: candidate, Score: Score(now, candidate)})
	}
	sort.SliceStable(remaining, func(left, right int) bool {
		if remaining[left].Score.Total != remaining[right].Score.Total {
			return remaining[left].Score.Total > remaining[right].Score.Total
		}
		if !remaining[left].PublishedAt.Equal(remaining[right].PublishedAt) {
			return remaining[left].PublishedAt.After(remaining[right].PublishedAt)
		}
		return remaining[left].EntryID < remaining[right].EntryID
	})
	if limit > len(remaining) {
		limit = len(remaining)
	}
	result := make([]ScoredCandidate, 0, limit)
	selectedItems := make([]ScoredCandidate, 0, len(previousSources)+limit)
	for _, sourceID := range previousSources {
		selectedItems = append(selectedItems, ScoredCandidate{Candidate: Candidate{SourceID: sourceID}})
	}
	sourceCounts := make(map[string]int)
	topicCounts := make(map[string]int)
	for len(result) < limit && len(remaining) > 0 {
		strictCounts := enforceFirstTwentyCounts && len(result) < 20
		selected := eligibleIndex(selectedItems, remaining, sourceCounts, topicCounts, strictCounts)
		if selected < 0 && strictCounts {
			selected = eligibleIndex(selectedItems, remaining, sourceCounts, topicCounts, false)
		}
		if selected < 0 {
			break
		}
		item := remaining[selected]
		result = append(result, item)
		selectedItems = append(selectedItems, item)
		sourceCounts[item.SourceID]++
		if item.PrimaryTopicID != "" {
			topicCounts[item.PrimaryTopicID]++
		}
		remaining = append(remaining[:selected], remaining[selected+1:]...)
	}
	return result
}

func eligibleIndex(selected, remaining []ScoredCandidate, sourceCounts, topicCounts map[string]int, strictCounts bool) int {
	for index, candidate := range remaining {
		if len(selected) >= 2 && selected[len(selected)-1].SourceID == candidate.SourceID && selected[len(selected)-2].SourceID == candidate.SourceID {
			continue
		}
		if strictCounts && sourceCounts[candidate.SourceID] >= 3 {
			continue
		}
		if strictCounts && candidate.PrimaryTopicID != "" && topicCounts[candidate.PrimaryTopicID] >= 5 {
			continue
		}
		return index
	}
	return -1
}

func clamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
