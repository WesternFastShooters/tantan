package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	languagePattern   = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
)

type EnrichmentV1 struct {
	Version          int      `json:"version"`
	DetectedLanguage string   `json:"detectedLanguage"`
	TitleZh          *string  `json:"titleZh"`
	ContentZh        *string  `json:"contentZh"`
	SummaryZh        string   `json:"summaryZh"`
	KeyPoints        []string `json:"keyPoints"`
}

type TopicClassificationV1 struct {
	Version int                   `json:"version"`
	Topics  []TopicClassification `json:"topics"`
}

type TopicClassification struct {
	TopicID    string  `json:"topicId"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func ValidateEnrichmentOutput(contents []byte) (EnrichmentV1, error) {
	required := []string{"version", "detectedLanguage", "titleZh", "contentZh", "summaryZh", "keyPoints"}
	if err := validateExactObject(contents, required); err != nil {
		return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
	}
	var output EnrichmentV1
	if err := decodeStrict(contents, &output); err != nil {
		return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
	}
	if output.Version != 1 || !languagePattern.MatchString(output.DetectedLanguage) {
		return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
	}
	if output.TitleZh != nil && utf8.RuneCountInString(*output.TitleZh) > 500 {
		return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
	}
	if output.ContentZh != nil && utf8.RuneCountInString(*output.ContentZh) > 100_000 {
		return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
	}
	if count := utf8.RuneCountInString(output.SummaryZh); count < 1 || count > 2_000 {
		return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
	}
	if len(output.KeyPoints) < 1 || len(output.KeyPoints) > 8 {
		return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
	}
	seen := make(map[string]struct{}, len(output.KeyPoints))
	for _, point := range output.KeyPoints {
		if count := utf8.RuneCountInString(point); count < 1 || count > 300 {
			return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
		}
		if _, duplicate := seen[point]; duplicate {
			return EnrichmentV1{}, errors.New("AI enrichment output does not match schema")
		}
		seen[point] = struct{}{}
	}
	return output, nil
}

func ValidateTopicOutput(contents []byte) (TopicClassificationV1, error) {
	if err := validateExactObject(contents, []string{"version", "topics"}); err != nil {
		return TopicClassificationV1{}, errors.New("AI topic output does not match schema")
	}
	var raw struct {
		Version int               `json:"version"`
		Topics  []json.RawMessage `json:"topics"`
	}
	if err := decodeStrict(contents, &raw); err != nil || raw.Version != 1 || len(raw.Topics) < 1 || len(raw.Topics) > 5 {
		return TopicClassificationV1{}, errors.New("AI topic output does not match schema")
	}
	output := TopicClassificationV1{Version: raw.Version, Topics: make([]TopicClassification, 0, len(raw.Topics))}
	seen := make(map[string]struct{}, len(raw.Topics))
	for _, item := range raw.Topics {
		if err := validateExactObject(item, []string{"topicId", "confidence", "reason"}); err != nil {
			return TopicClassificationV1{}, errors.New("AI topic output does not match schema")
		}
		var topic TopicClassification
		if err := decodeStrict(item, &topic); err != nil {
			return TopicClassificationV1{}, errors.New("AI topic output does not match schema")
		}
		if count := utf8.RuneCountInString(topic.TopicID); count < 1 || count > 128 || !identifierPattern.MatchString(topic.TopicID) || topic.Confidence < 0 || topic.Confidence > 1 {
			return TopicClassificationV1{}, errors.New("AI topic output does not match schema")
		}
		if count := utf8.RuneCountInString(topic.Reason); count < 1 || count > 200 {
			return TopicClassificationV1{}, errors.New("AI topic output does not match schema")
		}
		if _, duplicate := seen[topic.TopicID]; duplicate {
			return TopicClassificationV1{}, errors.New("AI topic output contains duplicate topics")
		}
		seen[topic.TopicID] = struct{}{}
		output.Topics = append(output.Topics, topic)
	}
	return output, nil
}

func validateExactObject(contents []byte, required []string) error {
	var object map[string]json.RawMessage
	if err := decodeStrict(contents, &object); err != nil || len(object) != len(required) {
		return errors.New("object keys differ")
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return errors.New("required key is missing")
		}
	}
	return nil
}

func decodeStrict(contents []byte, target any) error {
	if len(bytes.TrimSpace(contents)) == 0 || len(contents) > 4*1024*1024 || !utf8.Valid(contents) {
		return errors.New("JSON document size is invalid")
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

func safePromptText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
