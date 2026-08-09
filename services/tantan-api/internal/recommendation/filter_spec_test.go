package recommendation_test

import (
	"bytes"
	"testing"

	"tantan.local/tantan-api/internal/recommendation"
)

func validFilterSpec() []byte {
	return []byte(`{"version":1,"windowDays":7,"includeTopics":["topic_ai"],"excludeTopics":[],"includeSources":["feed_1"],"excludeSources":[],"includeTerms":["Claude Code"],"negativeTerms":["融资"],"languages":["en","zh-CN"],"contentStyles":["article","post"],"weights":{"freshness":1.2,"topicMatch":1.5,"sourceAffinity":1,"quality":0.8,"diversity":1}}`)
}

func TestFilterSpecStrictlyMatchesApprovedSchemaAndCanonicalizes(t *testing.T) {
	spec, canonical, err := recommendation.ValidateFilterSpec(validFilterSpec())
	if err != nil || spec.Version != 1 || spec.WindowDays != 7 || len(spec.IncludeTerms) != 1 {
		t.Fatalf("spec=%#v canonical=%s err=%v", spec, canonical, err)
	}
	_, second, err := recommendation.ValidateFilterSpec(canonical)
	if err != nil || !bytes.Equal(canonical, second) {
		t.Fatalf("canonical is unstable: first=%s second=%s err=%v", canonical, second, err)
	}
	invalid := [][]byte{
		[]byte(`{"version":1}`),
		append(append([]byte(nil), validFilterSpec()[:len(validFilterSpec())-1]...), []byte(`,"extra":true}`)...),
		bytes.Replace(validFilterSpec(), []byte(`"windowDays":7`), []byte(`"windowDays":31`), 1),
		bytes.Replace(validFilterSpec(), []byte(`"includeTopics":["topic_ai"]`), []byte(`"includeTopics":["bad id"]`), 1),
		bytes.Replace(validFilterSpec(), []byte(`"includeTerms":["Claude Code"]`), []byte(`"includeTerms":["same","same"]`), 1),
		bytes.Replace(validFilterSpec(), []byte(`"freshness":1.2`), []byte(`"freshness":3`), 1),
	}
	for index, raw := range invalid {
		if _, _, err := recommendation.ValidateFilterSpec(raw); err == nil {
			t.Fatalf("invalid filter spec %d accepted: %s", index, raw)
		}
	}
}
