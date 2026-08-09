package schema_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGeneratedAISchemasMatchApprovedSnapshots(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", ".."))

	for _, name := range []string{
		"ai-enrichment-v1.schema.json",
		"filter-spec-v1.schema.json",
		"home-response.schema.json",
		"topic-classification-v1.schema.json",
	} {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(repositoryRoot, "spec-package", "schemas", name))
			if err != nil {
				t.Fatalf("read approved schema: %v", err)
			}
			generated, err := os.ReadFile(filepath.Join(filepath.Dir(filename), name))
			if err != nil {
				t.Fatalf("read generated schema: %v", err)
			}
			if string(generated) != string(source) {
				t.Fatal("generated schema differs byte-for-byte from approved schema")
			}
			var document map[string]any
			if err := json.Unmarshal(generated, &document); err != nil {
				t.Fatalf("parse generated schema: %v", err)
			}
		})
	}
}
