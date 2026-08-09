package ops_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/storage"
)

func TestDoctorReportsStableRedactedChecksAndRecovery(t *testing.T) {
	const canary = "doctor-CANARY-token-key-prompt-content"
	dataDirectory := t.TempDir()
	doctor, err := ops.NewDoctor(ops.DoctorConfig{
		DataDir:  dataDirectory,
		Address:  "127.0.0.1:3000",
		Keychain: &probeKeychain{values: map[string]string{}, err: errors.New(canary)},
		Timeout:  time.Second,
		PortProbe: func(context.Context, string) error {
			return errors.New(canary)
		},
		DNSProbe: func(context.Context, string) error {
			return errors.New(canary)
		},
		TLSProbe: func(context.Context, string) error {
			return errors.New(canary)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Run(context.Background())
	if report.OK {
		t.Fatalf("faulted doctor=%#v", report)
	}
	wantNames := []string{"port", "data_directory", "sqlite", "migrations", "keychain", "dns", "tls"}
	if len(report.Checks) != len(wantNames) {
		t.Fatalf("checks=%#v", report.Checks)
	}
	for index, check := range report.Checks {
		if check.Name != wantNames[index] || (check.Status == "error" && strings.TrimSpace(check.Recovery) == "") {
			t.Fatalf("check[%d]=%#v", index, check)
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), canary) || strings.Contains(string(encoded), dataDirectory) {
		t.Fatalf("doctor leaked sensitive input: %s", encoded)
	}
}

func TestDoctorReportsSQLiteLockWithStableRecovery(t *testing.T) {
	ctx := context.Background()
	dataDirectory := t.TempDir()
	store, err := storage.Open(ctx, storage.Config{DataDir: dataDirectory})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	transaction, err := store.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, "INSERT INTO accounts(user_id,name,timezone,created_at,updated_at) VALUES('locked','Locked','Asia/Shanghai','2026-08-09T12:00:00Z','2026-08-09T12:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	doctor, err := ops.NewDoctor(ops.DoctorConfig{
		DataDir:  dataDirectory,
		Address:  "127.0.0.1:3000",
		Keychain: &probeKeychain{values: map[string]string{}},
		Timeout:  100 * time.Millisecond,
		PortProbe: func(context.Context, string) error {
			return nil
		},
		DNSProbe: func(context.Context, string) error {
			return nil
		},
		TLSProbe: func(context.Context, string) error {
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	checkContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	report := doctor.Run(checkContext)
	if report.OK {
		t.Fatalf("locked database doctor=%#v", report)
	}
	checks := map[string]ops.DoctorCheck{}
	for _, check := range report.Checks {
		checks[check.Name] = check
	}
	if checks["sqlite"].Status != "error" || strings.TrimSpace(checks["sqlite"].Recovery) == "" {
		t.Fatalf("sqlite check=%#v", checks["sqlite"])
	}
	if checks["migrations"].Status != "ok" {
		t.Fatalf("migrations check=%#v", checks["migrations"])
	}
}
