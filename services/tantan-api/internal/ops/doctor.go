package ops

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"tantan.local/tantan-api/internal/storage"
)

const doctorUpstreamHost = "api.folo.is"

type DoctorProbe func(context.Context, string) error

type DoctorConfig struct {
	DataDir   string
	Address   string
	Keychain  Keychain
	Timeout   time.Duration
	PortProbe DoctorProbe
	DNSProbe  DoctorProbe
	TLSProbe  DoctorProbe
}

type Doctor struct {
	config DoctorConfig
}

type DoctorCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Recovery string `json:"recovery,omitempty"`
}

type DoctorReport struct {
	OK     bool          `json:"ok"`
	Checks []DoctorCheck `json:"checks"`
}

func NewDoctor(config DoctorConfig) (*Doctor, error) {
	if config.DataDir == "" || config.Address == "" || config.Keychain == nil {
		return nil, errors.New("doctor data directory, address and Keychain are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultProbeTimeout
	}
	if config.PortProbe == nil {
		config.PortProbe = probeAvailablePort
	}
	if config.DNSProbe == nil {
		config.DNSProbe = probePublicDNS
	}
	if config.TLSProbe == nil {
		config.TLSProbe = probeTLS
	}
	return &Doctor{config: config}, nil
}

func (doctor *Doctor) Run(ctx context.Context) DoctorReport {
	report := DoctorReport{OK: true, Checks: make([]DoctorCheck, 0, 7)}
	report.add(runDoctorProbe(ctx, doctor.config.Timeout, "port", "loopback port is available", "stop the process using 127.0.0.1:3000 and retry", func(probeContext context.Context) error {
		return doctor.config.PortProbe(probeContext, doctor.config.Address)
	}))
	report.add(doctor.checkDataDirectory())
	store, openErr := storage.Open(ctx, storage.Config{DataDir: doctor.config.DataDir})
	if openErr != nil {
		report.add(errorDoctorCheck("sqlite", "SQLite is unavailable", "verify the data directory permissions and restore a valid backup"))
		report.add(errorDoctorCheck("migrations", "database migrations are unavailable", "restore a database created by this Tantan build"))
	} else {
		integrity, err := store.Integrity(ctx)
		if err != nil || integrity != "ok" || probeSQLiteWritable(ctx, store.DB(), doctor.config.Timeout) != nil {
			report.add(errorDoctorCheck("sqlite", "SQLite integrity or write check failed", "stop Tantan and restore the latest verified backup"))
		} else {
			report.add(okDoctorCheck("sqlite", "SQLite integrity is ok"))
		}
		if CheckMigrations(ctx, store.DB()) != nil {
			report.add(errorDoctorCheck("migrations", "database migration checksum failed", "restore a database created by this Tantan build"))
		} else {
			report.add(okDoctorCheck("migrations", "database migrations are current"))
		}
		_ = store.Close()
	}
	report.add(runDoctorProbe(ctx, doctor.config.Timeout, "keychain", "Keychain set/get/delete probe passed", "unlock the OS Keychain and allow Tantan access", func(probeContext context.Context) error {
		return ProbeKeychain(probeContext, doctor.config.Keychain)
	}))
	report.add(runDoctorProbe(ctx, doctor.config.Timeout, "dns", "Folo DNS resolves to usable addresses", "check DNS and network access to the built-in Folo host", func(probeContext context.Context) error {
		return doctor.config.DNSProbe(probeContext, doctorUpstreamHost)
	}))
	report.add(runDoctorProbe(ctx, doctor.config.Timeout, "tls", "Folo TLS handshake passed", "check system time, certificates and HTTPS access to the built-in Folo host", func(probeContext context.Context) error {
		return doctor.config.TLSProbe(probeContext, doctorUpstreamHost)
	}))
	for _, check := range report.Checks {
		if check.Status != "ok" {
			report.OK = false
		}
	}
	return report
}

func probeSQLiteWritable(ctx context.Context, database interface {
	Conn(context.Context) (*sql.Conn, error)
}, timeout time.Duration) error {
	probeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	connection, err := database.Conn(probeContext)
	if err != nil {
		return errors.New("SQLite connection probe failed")
	}
	defer connection.Close()
	if _, err := connection.ExecContext(probeContext, "PRAGMA busy_timeout=100"); err != nil {
		return errors.New("SQLite timeout probe failed")
	}
	if _, err := connection.ExecContext(probeContext, "BEGIN IMMEDIATE"); err != nil {
		return errors.New("SQLite write probe failed")
	}
	if _, err := connection.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		return errors.New("SQLite rollback probe failed")
	}
	return nil
}

func (doctor *Doctor) checkDataDirectory() DoctorCheck {
	path, err := filepath.Abs(doctor.config.DataDir)
	if err != nil {
		return errorDoctorCheck("data_directory", "data directory is invalid", "choose a local writable data directory")
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return errorDoctorCheck("data_directory", "data directory is not writable", "choose a local writable data directory")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return errorDoctorCheck("data_directory", "data directory permissions are unsafe", "set the data directory permissions to 0700")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return errorDoctorCheck("data_directory", "data directory permissions are unsafe", "set the data directory permissions to 0700")
	}
	return okDoctorCheck("data_directory", "data directory permissions are 0700")
}

func (report *DoctorReport) add(check DoctorCheck) {
	report.Checks = append(report.Checks, check)
}

func runDoctorProbe(ctx context.Context, timeout time.Duration, name, success, recovery string, probe func(context.Context) error) DoctorCheck {
	probeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if probe(probeContext) != nil {
		return errorDoctorCheck(name, name+" check failed", recovery)
	}
	return okDoctorCheck(name, success)
}

func okDoctorCheck(name, message string) DoctorCheck {
	return DoctorCheck{Name: name, Status: "ok", Message: message}
}

func errorDoctorCheck(name, message, recovery string) DoctorCheck {
	return DoctorCheck{Name: name, Status: "error", Message: message, Recovery: recovery}
}

func probeAvailablePort(ctx context.Context, address string) error {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return errors.New("port is occupied")
	}
	return listener.Close()
}

func probePublicDNS(ctx context.Context, host string) error {
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return errors.New("DNS lookup failed")
	}
	for _, address := range addresses {
		if err := validateBuiltInHostAddress(address.IP); err != nil {
			return errors.New("DNS returned an unusable address")
		}
	}
	return nil
}

func probeTLS(ctx context.Context, host string) error {
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return errors.New("TLS DNS lookup failed")
	}
	dialer := &net.Dialer{}
	for _, address := range addresses {
		if err := validateBuiltInHostAddress(address.IP); err != nil {
			return errors.New("TLS DNS returned an unusable address")
		}
		connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address.IP.String(), "443"))
		if err != nil {
			continue
		}
		tlsConnection := tls.Client(connection, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host})
		err = tlsConnection.HandshakeContext(ctx)
		_ = tlsConnection.Close()
		if err == nil {
			return nil
		}
	}
	return errors.New("TLS handshake failed")
}

// doctorUpstreamHost is a compile-time constant, so accepting the RFC 2544
// benchmark range used by local transparent proxies cannot turn this probe
// into a caller-controlled SSRF primitive. Private, loopback and link-local
// destinations remain forbidden.
func validateBuiltInHostAddress(ip net.IP) error {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return errors.New("invalid address")
	}
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return errors.New("unusable address")
	}
	return nil
}
