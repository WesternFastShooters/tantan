package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestEmbeddedMigrationsMatchApprovedSpecPackageSnapshots(t *testing.T) {
	expected := map[string]string{
		"0001_core.sql":             "3ba2b6f9a9c871fe06bdeaa4625c7f1e59aed85508113f2ae0b1f4275b15d4ef",
		"0002_search_fts.sql":       "b0fc9c33a5b8819685236882cbbfcfc4a16b699b4154b3f0e046a4bd1a6a9db5",
		"0003_seed_core_topics.sql": "ba89ba40d804a05ab5a32d57e490e5d0b4bf728eb76710673fd3d0f897b97b51",
		"0004_mobile_web_v2.sql":    "1ce30c907bdceab4a5c0230aa668a31ebc3e70b62a4f7a9c092d7483330ad527",
	}
	for name, want := range expected {
		contents, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		digest := sha256.Sum256(contents)
		if got := hex.EncodeToString(digest[:]); got != want {
			t.Fatalf("%s digest=%s want=%s", name, got, want)
		}
	}
}
