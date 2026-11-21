package logger_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// memSink collects the records a logger writes. A file sink would do the same
// job, but Windows keeps a file handle locked for the life of the process, so
// t.TempDir's cleanup failed with "the process cannot access the file" and
// turned four passing assertions into failures.
type memSink struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *memSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.buf.Write(p)
}

func (s *memSink) Sync() error { return nil }

// lines returns the non-empty records written so far.
func (s *memSink) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []string
	for line := range strings.SplitSeq(s.buf.String(), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}

	return out
}

// buildRecordLogger returns a logger that writes into memory but otherwise
// takes the production configuration as-is: the same encoder, level and
// sampler, wrapped in the same redacting core logger.New applies. Building the
// core here rather than through cfg.Build is what keeps the records out of a
// file; the sampler below is the one cfg.Build would install, from the same
// cfg.Sampling numbers.
func buildRecordLogger(t *testing.T, cfg zap.Config) (logger.Logger, *memSink) {
	t.Helper()

	if cfg.Encoding != "json" {
		t.Fatalf("these tests read JSON records, got encoding %q", cfg.Encoding)
	}
	if cfg.Level == (zap.AtomicLevel{}) {
		t.Fatal("the config carries no level; the test would prove nothing")
	}

	sink := &memSink{}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(cfg.EncoderConfig), sink, cfg.Level)
	if cfg.Sampling != nil {
		core = zapcore.NewSamplerWithOptions(core, time.Second, cfg.Sampling.Initial, cfg.Sampling.Thereafter)
	}

	return logger.NewWithCore(core), sink
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

	log, sink := buildRecordLogger(t, cfg)

	const total = 300
	for range total {
		log.Warn("identical record", zap.String("access_token", "s3cr3t-value"))
	}

	lines := sink.lines()
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
	log, sink := buildRecordLogger(t, zap.NewProductionConfig())

	const total = 300
	for range total {
		log.Warn("identical record",
			zap.String("access_token", "s3cr3t-value"),
			zap.String("client_secret", "another-secret"),
			zap.String("user_agent", "grpc-go/1.82.1"),
		)
	}

	lines := sink.lines()
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
	log, sink := buildRecordLogger(t, zap.NewProductionConfig())

	log.With(zap.String("refresh_token", "rt-secret")).Warn("derived record")

	lines := sink.lines()
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

	log, sink := buildRecordLogger(t, cfg)

	log.Debug("must not be written", zap.String("access_token", "s3cr3t-value"))
	if lines := sink.lines(); len(lines) != 0 {
		t.Errorf("want no record below the level, got %d: %v", len(lines), lines)
	}

	log.Info("must be written")
	if lines := sink.lines(); len(lines) != 1 {
		t.Errorf("want the Info record, got %d", len(lines))
	}
}
