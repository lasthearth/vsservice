package logger_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

// buildFileLogger returns a logger on the production config path, so the sampler
// zap.Config.Build installs sits under the redacting core exactly as it does in
// the service. Records land in one file the test counts.
func buildFileLogger(t *testing.T, cfg zap.Config) (logger.Logger, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "log.json")
	cfg.OutputPaths = []string{path}
	cfg.ErrorOutputPaths = []string{filepath.Join(dir, "error.json")}

	log, err := logger.New(&cfg)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log, path
}

func readLines(t *testing.T, path string) []string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	var out []string
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scan.Scan() {
		if line := scan.Text(); strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return out
}

// The redacting core used to answer Check from Enabled alone and add itself, so
// the sampler underneath never counted and never dropped: 300 identical records
// wrote 300 times. Delegating Check restores the production policy of 100 then
// 1 in 100, which is 102 records inside one tick.
func TestRedactingCoreKeepsZapSampling(t *testing.T) {
	cfg := zap.NewProductionConfig()
	if cfg.Sampling == nil {
		t.Fatal("the production config must sample; the test would prove nothing")
	}
	if cfg.Sampling.Initial != 100 || cfg.Sampling.Thereafter != 100 {
		t.Fatalf("want the 100:100 production policy, got %d:%d", cfg.Sampling.Initial, cfg.Sampling.Thereafter)
	}

	log, path := buildFileLogger(t, cfg)

	const total = 300
	for range total {
		log.Warn("identical record", zap.String("access_token", "s3cr3t-value"))
	}
	if err := log.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	lines := readLines(t, path)
	got := len(lines)
	if got >= total {
		t.Errorf("want the sampler to drop records, got all %d", got)
	}
	// 102 is one tick: 100 initial plus every 100th of the remaining 200. A
	// tick boundary inside the loop can only add another tick's worth.
	if got < 102 || got > 204 {
		t.Errorf("want 102 sampled records, got %d", got)
	}
	t.Logf("300 identical Warn records wrote %d lines", got)
}

// Sampling must not weaken masking: every record that survives still carries
// [redacted] and never the credential.
func TestRedactingCoreMasksWhatTheSamplerLetsThrough(t *testing.T) {
	log, path := buildFileLogger(t, zap.NewProductionConfig())

	const total = 300
	for range total {
		log.Warn("identical record",
			zap.String("access_token", "s3cr3t-value"),
			zap.String("client_secret", "another-secret"),
			zap.String("user_agent", "grpc-go/1.82.1"),
		)
	}
	if err := log.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) == 0 {
		t.Fatal("want at least one sampled record to inspect")
	}
	for i, line := range lines {
		if strings.Contains(line, "s3cr3t-value") || strings.Contains(line, "another-secret") {
			t.Fatalf("line %d leaks a credential: %s", i, line)
		}
		if !strings.Contains(line, `"access_token":"[redacted]"`) {
			t.Fatalf("line %d has no masked access_token: %s", i, line)
		}
		if !strings.Contains(line, `"client_secret":"[redacted]"`) {
			t.Fatalf("line %d has no masked client_secret: %s", i, line)
		}
		if !strings.Contains(line, `"user_agent":"grpc-go/1.82.1"`) {
			t.Fatalf("line %d dropped a normal field: %s", i, line)
		}
	}
}

// Fields attached with With are masked before the sampler sees them, so a
// derived logger keeps the guarantee.
func TestRedactingCoreMasksFieldsAttachedWithWith(t *testing.T) {
	log, path := buildFileLogger(t, zap.NewProductionConfig())

	log.With(zap.String("refresh_token", "rt-secret")).Warn("derived record")
	if err := log.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("want 1 record, got %d", len(lines))
	}
	if strings.Contains(lines[0], "rt-secret") {
		t.Errorf("the derived logger leaked the token: %s", lines[0])
	}
	if !strings.Contains(lines[0], `"refresh_token":"[redacted]"`) {
		t.Errorf("want the token masked, got %s", lines[0])
	}
}

// Delegating Check must not turn the inner core's level filter off: a Debug
// record under an Info level still writes nothing.
func TestRedactingCoreStillFiltersDisabledLevels(t *testing.T) {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)

	log, path := buildFileLogger(t, cfg)

	log.Debug("must not be written", zap.String("access_token", "s3cr3t-value"))
	if err := log.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if lines := readLines(t, path); len(lines) != 0 {
		t.Errorf("want no record below the level, got %d: %v", len(lines), lines)
	}

	log.Info("must be written")
	if err := log.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if lines := readLines(t, path); len(lines) != 1 {
		t.Errorf("want the Info record, got %d", len(lines))
	}
}
