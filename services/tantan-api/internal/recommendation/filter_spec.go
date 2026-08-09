package recommendation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"unicode/utf8"
)

var (
	filterIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
	filterLanguagePattern   = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)
)

type FilterSpecV1 struct {
	Version        int      `json:"version"`
	WindowDays     int      `json:"windowDays"`
	IncludeTopics  []string `json:"includeTopics"`
	ExcludeTopics  []string `json:"excludeTopics"`
	IncludeSources []string `json:"includeSources"`
	ExcludeSources []string `json:"excludeSources"`
	IncludeTerms   []string `json:"includeTerms"`
	NegativeTerms  []string `json:"negativeTerms"`
	Languages      []string `json:"languages"`
	ContentStyles  []string `json:"contentStyles"`
	Weights        Weights  `json:"weights"`
}

func ValidateFilterSpec(contents []byte) (FilterSpecV1, []byte, error) {
	required := []string{"version", "windowDays", "includeTopics", "excludeTopics", "includeSources", "excludeSources", "includeTerms", "negativeTerms", "languages", "contentStyles", "weights"}
	if err := exactJSONObject(contents, required); err != nil {
		return FilterSpecV1{}, nil, errors.New("AI filter output does not match schema")
	}
	var raw struct {
		Version        int             `json:"version"`
		WindowDays     int             `json:"windowDays"`
		IncludeTopics  []string        `json:"includeTopics"`
		ExcludeTopics  []string        `json:"excludeTopics"`
		IncludeSources []string        `json:"includeSources"`
		ExcludeSources []string        `json:"excludeSources"`
		IncludeTerms   []string        `json:"includeTerms"`
		NegativeTerms  []string        `json:"negativeTerms"`
		Languages      []string        `json:"languages"`
		ContentStyles  []string        `json:"contentStyles"`
		Weights        json.RawMessage `json:"weights"`
	}
	if err := decodeFilterJSON(contents, &raw); err != nil || raw.Version != 1 || raw.WindowDays < 1 || raw.WindowDays > 30 {
		return FilterSpecV1{}, nil, errors.New("AI filter output does not match schema")
	}
	if err := exactJSONObject(raw.Weights, []string{"freshness", "topicMatch", "sourceAffinity", "quality", "diversity"}); err != nil {
		return FilterSpecV1{}, nil, errors.New("AI filter output does not match schema")
	}
	var weights Weights
	if err := decodeFilterJSON(raw.Weights, &weights); err != nil || !validWeight(weights.Freshness) || !validWeight(weights.TopicMatch) || !validWeight(weights.SourceAffinity) || !validWeight(weights.Quality) || !validWeight(weights.Diversity) {
		return FilterSpecV1{}, nil, errors.New("AI filter output does not match schema")
	}
	if !validIdentifiers(raw.IncludeTopics, 50) || !validIdentifiers(raw.ExcludeTopics, 50) || !validIdentifiers(raw.IncludeSources, 50) || !validIdentifiers(raw.ExcludeSources, 50) || !validTerms(raw.IncludeTerms) || !validTerms(raw.NegativeTerms) || !validLanguages(raw.Languages) || !validStyles(raw.ContentStyles) {
		return FilterSpecV1{}, nil, errors.New("AI filter output does not match schema")
	}
	spec := FilterSpecV1{
		Version:        raw.Version,
		WindowDays:     raw.WindowDays,
		IncludeTopics:  nonNil(raw.IncludeTopics),
		ExcludeTopics:  nonNil(raw.ExcludeTopics),
		IncludeSources: nonNil(raw.IncludeSources),
		ExcludeSources: nonNil(raw.ExcludeSources),
		IncludeTerms:   nonNil(raw.IncludeTerms),
		NegativeTerms:  nonNil(raw.NegativeTerms),
		Languages:      nonNil(raw.Languages),
		ContentStyles:  nonNil(raw.ContentStyles),
		Weights:        weights,
	}
	canonical, err := json.Marshal(spec)
	if err != nil {
		return FilterSpecV1{}, nil, errors.New("canonicalize AI filter output")
	}
	return spec, canonical, nil
}

func exactJSONObject(contents []byte, required []string) error {
	var object map[string]json.RawMessage
	if err := decodeFilterJSON(contents, &object); err != nil || len(object) != len(required) {
		return errors.New("object keys differ")
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return errors.New("required key is missing")
		}
	}
	return nil
}

func decodeFilterJSON(contents []byte, target any) error {
	if len(bytes.TrimSpace(contents)) == 0 || len(contents) > 4*1024*1024 || !utf8.Valid(contents) {
		return errors.New("JSON document size or encoding is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("JSON document contains trailing data")
	}
	return nil
}

func validIdentifiers(values []string, maximum int) bool {
	if values == nil || len(values) > maximum {
		return false
	}
	return uniqueStrings(values, func(value string) bool {
		count := utf8.RuneCountInString(value)
		return count >= 1 && count <= 128 && filterIdentifierPattern.MatchString(value)
	})
}

func validTerms(values []string) bool {
	if values == nil || len(values) > 30 {
		return false
	}
	return uniqueStrings(values, func(value string) bool {
		count := utf8.RuneCountInString(value)
		return count >= 1 && count <= 80
	})
}

func validLanguages(values []string) bool {
	return values != nil && len(values) <= 10 && uniqueStrings(values, filterLanguagePattern.MatchString)
}

func validStyles(values []string) bool {
	allowed := map[string]bool{"article": true, "post": true, "image": true, "video": true}
	return values != nil && len(values) <= 4 && uniqueStrings(values, func(value string) bool { return allowed[value] })
}

func uniqueStrings(values []string, validate func(string) bool) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validate(value) {
			return false
		}
		if _, duplicate := seen[value]; duplicate {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validWeight(value float64) bool {
	return value >= 0 && value <= 2
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
