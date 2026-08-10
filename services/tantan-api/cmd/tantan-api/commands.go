package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"tantan.local/tantan-api/internal/keyring"
	"tantan.local/tantan-api/internal/ops"
	"tantan.local/tantan-api/internal/storage"
)

const readinessKeychainService = "tantan.readiness.probe"

var errDoctorChecksFailed = errors.New("doctor found unavailable dependencies")

func runManagementCommand(ctx context.Context, arguments []string, output io.Writer) (bool, error) {
	if len(arguments) == 0 {
		return false, nil
	}
	switch arguments[0] {
	case "migrate":
		return true, runMigrateCommand(ctx, arguments[1:])
	case "doctor":
		return true, runDoctorCommand(ctx, arguments[1:], output)
	case "backup":
		return true, runBackupCommand(ctx, arguments[1:], output)
	case "restore":
		return true, runRestoreCommand(ctx, arguments[1:], output)
	case "serve":
		return false, nil
	default:
		return false, nil
	}
}

func runMigrateCommand(ctx context.Context, arguments []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataDirectory := flags.String("data-dir", configuredDataDirectory(), "local Tantan data directory")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return errors.New("migrate accepts no positional arguments")
	}
	store, err := storage.Open(ctx, storage.Config{DataDir: *dataDirectory})
	if err != nil {
		return err
	}
	return store.Close()
}

func runDoctorCommand(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataDirectory := flags.String("data-dir", configuredDataDirectory(), "local Tantan data directory")
	address := flags.String("listen-address", configuredListenAddress(), "loopback listen address")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 {
		return errors.New("invalid doctor arguments")
	}
	probeStore, err := keyring.NewOSStore(readinessKeychainService)
	if err != nil {
		return err
	}
	doctor, err := ops.NewDoctor(ops.DoctorConfig{DataDir: *dataDirectory, Address: *address, Keychain: probeStore, Timeout: 5 * time.Second})
	if err != nil {
		return err
	}
	report := doctor.Run(ctx)
	if err := writeCommandJSON(output, report); err != nil {
		return err
	}
	if !report.OK {
		return errDoctorChecksFailed
	}
	return nil
}

func runBackupCommand(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataDirectory := flags.String("data-dir", configuredDataDirectory(), "local Tantan data directory")
	backupOutput := flags.String("output", "", "explicit non-existing backup path")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *backupOutput == "" {
		return errors.New("backup requires --output and accepts no positional arguments")
	}
	store, err := storage.Open(ctx, storage.Config{DataDir: *dataDirectory})
	if err != nil {
		return err
	}
	defer store.Close()
	result, err := ops.Backup(ctx, store, *backupOutput)
	if err != nil {
		return err
	}
	return writeCommandJSON(output, result)
}

func runRestoreCommand(ctx context.Context, arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dataDirectory := flags.String("data-dir", configuredDataDirectory(), "local Tantan data directory")
	input := flags.String("input", "", "verified SQLite backup path")
	if err := flags.Parse(arguments); err != nil || flags.NArg() != 0 || *input == "" {
		return errors.New("restore requires --input and accepts no positional arguments")
	}
	listener, err := net.Listen("tcp", configuredListenAddress())
	if err != nil {
		return ops.ErrServiceRunning
	}
	if err := listener.Close(); err != nil {
		return errors.New("verify stopped service failed")
	}
	result, err := ops.Restore(ctx, *input, *dataDirectory)
	if err != nil {
		return err
	}
	return writeCommandJSON(output, result)
}

func configuredDataDirectory() string {
	if value := os.Getenv("TANTAN_DATA_DIR"); value != "" {
		return value
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "Tantan")
	}
	return filepath.Join(root, "Tantan")
}

func configuredListenAddress() string {
	if value := os.Getenv("TANTAN_LISTEN_ADDR"); value != "" {
		return value
	}
	return defaultListenAddress
}

func writeCommandJSON(output io.Writer, value any) error {
	if output == nil {
		return errors.New("command output is required")
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(value); err != nil {
		return errors.New("write command output failed")
	}
	return nil
}
