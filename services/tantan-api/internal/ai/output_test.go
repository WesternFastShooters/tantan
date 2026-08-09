package ai_test

import (
	"testing"

	"tantan.local/tantan-api/internal/ai"
)

func TestEnrichmentOutputStrictlyFollowsApprovedSchema(t *testing.T) {
	valid := []byte(`{"version":1,"detectedLanguage":"en","titleZh":"标题","contentZh":"正文","summaryZh":"摘要","keyPoints":["要点一","要点二"]}`)
	result, err := ai.ValidateEnrichmentOutput(valid)
	if err != nil || result.Version != 1 || len(result.KeyPoints) != 2 {
		t.Fatalf("valid output=%#v err=%v", result, err)
	}
	invalid := [][]byte{
		[]byte(`{"version":1,"detectedLanguage":"en","titleZh":null,"contentZh":null,"summaryZh":"摘要","keyPoints":["要点"],"extra":true}`),
		[]byte(`{"version":1,"detectedLanguage":"en","contentZh":null,"summaryZh":"摘要","keyPoints":["要点"]}`),
		[]byte(`{"version":1,"detectedLanguage":"invalid_language","titleZh":null,"contentZh":null,"summaryZh":"摘要","keyPoints":["要点"]}`),
		[]byte(`{"version":1,"detectedLanguage":"en","titleZh":null,"contentZh":null,"summaryZh":"","keyPoints":[]}`),
		[]byte(`{"version":1,"detectedLanguage":"en","titleZh":null,"contentZh":null,"summaryZh":"摘要","keyPoints":["重复","重复"]}`),
		append([]byte(`{"version":1,"detectedLanguage":"en","titleZh":null,"contentZh":null,"summaryZh":"`), append([]byte{0xff}, []byte(`","keyPoints":["要点"]}`)...)...),
	}
	for index, contents := range invalid {
		if _, err := ai.ValidateEnrichmentOutput(contents); err == nil {
			t.Fatalf("invalid output %d was accepted", index)
		}
	}
}

func TestTopicOutputStrictlyFollowsApprovedSchema(t *testing.T) {
	valid := []byte(`{"version":1,"topics":[{"topicId":"topic_ai","confidence":0.98,"reason":"AI content"}]}`)
	result, err := ai.ValidateTopicOutput(valid)
	if err != nil || len(result.Topics) != 1 {
		t.Fatalf("valid topic output=%#v err=%v", result, err)
	}
	for _, contents := range [][]byte{
		[]byte(`{"version":1,"topics":[]}`),
		[]byte(`{"version":1,"topics":[{"topicId":"bad id","confidence":0.5,"reason":"x"}]}`),
		[]byte(`{"version":1,"topics":[{"topicId":"topic_ai","confidence":2,"reason":"x"}]}`),
		[]byte(`{"version":1,"topics":[{"topicId":"topic_ai","confidence":0.5,"reason":"x","extra":1}]}`),
	} {
		if _, err := ai.ValidateTopicOutput(contents); err == nil {
			t.Fatalf("invalid topic output was accepted: %s", contents)
		}
	}
}
