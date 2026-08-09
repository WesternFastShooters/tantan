package ai

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type ConnectionTestResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latencyMs"`
	Model     string `json:"model"`
}

// TestConnection calls a locked provider preset with the temporary credential.
// It deliberately has no storage dependency, so the supplied key cannot be persisted.
func TestConnection(ctx context.Context, apiKey string, transport http.RoundTripper, now func() time.Time) (ConnectionTestResult, error) {
	if now == nil {
		now = time.Now
	}
	client, err := NewProviderClient(ProviderClientConfig{Transport: transport})
	if err != nil {
		return ConnectionTestResult{}, err
	}
	started := now()
	output, err := client.Generate(ctx, apiKey, GenerationRequest{
		SchemaName:   ConnectionSchemaName,
		SystemPrompt: "Return one small JSON object only.",
		UserPrompt:   `{"ping":"tantan"}`,
	})
	if err != nil {
		return ConnectionTestResult{}, err
	}
	if len(output) == 0 {
		return ConnectionTestResult{}, errors.New("AI provider returned no content")
	}
	latency := now().Sub(started).Milliseconds()
	if latency < 0 {
		latency = 0
	}
	if latency > providerTimeout.Milliseconds() {
		latency = providerTimeout.Milliseconds()
	}
	return ConnectionTestResult{OK: true, LatencyMS: latency, Model: FixedModel}, nil
}
