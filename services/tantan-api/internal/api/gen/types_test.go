package gen_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var contractInputs = []string{
	"spec-package/api/openapi.json",
	"spec-package/api/folo-route-policy.json",
	"spec-package/schemas/ai-enrichment-v1.schema.json",
	"spec-package/schemas/filter-spec-v1.schema.json",
	"spec-package/schemas/home-response.schema.json",
	"spec-package/schemas/topic-classification-v1.schema.json",
	"spec-package/db/0001_core.sql",
	"spec-package/db/0002_search_fts.sql",
	"spec-package/db/0003_seed_core_topics.sql",
	"spec-package/db/0004_mobile_web_v2.sql",
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", ".."))
}

func contractDigest(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	for _, name := range contractInputs {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read contract input %s: %v", name, err)
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(contents)
		_, _ = hash.Write([]byte{0})
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func TestGeneratedGoTypesMatchApprovedContract(t *testing.T) {
	root := repositoryRoot(t)
	contents, err := os.ReadFile(filepath.Join(root, "services", "tantan-api", "internal", "api", "gen", "types.go"))
	if err != nil {
		t.Fatalf("read generated Go types: %v", err)
	}

	text := string(contents)
	for _, required := range []string{
		"type HomeResponse struct",
		"type HomeCard struct",
		"type TopicsResponse struct",
		"type FilterMutationResponse struct",
		"type AIProviderResponse struct",
		"type ErrorResponse struct",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("generated Go types missing %q", required)
		}
	}

	digestDeclaration := fmt.Sprintf("const ContractSHA256 = %q", contractDigest(t, root))
	if !strings.Contains(text, digestDeclaration) {
		t.Errorf("generated Go types missing current digest %q", digestDeclaration)
	}
}
