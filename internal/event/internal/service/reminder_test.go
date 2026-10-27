package service

import (
	"context"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// entryWithEvent returns the one record carrying an event key, failing when it
// is absent or ambiguous.
func entryWithEvent(t *testing.T, logs *observer.ObservedLogs, event string) observer.LoggedEntry {
	t.Helper()

	var found []observer.LoggedEntry
	for _, e := range logs.All() {
		for _, f := range e.Context {
			if f.Key == "event" && f.String == event {
				found = append(found, e)
				break
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly 1 record with event %q, got %d: %+v", event, len(found), logs.All())
	}
	return found[0]
}

func intOf(t *testing.T, e observer.LoggedEntry, key string) int64 {
	t.Helper()

	for _, f := range e.Context {
		if f.Key == key {
			return f.Integer
		}
	}
	t.Fatalf("record %q has no field %q: %+v", e.Message, key, e.Context)
	return 0
}

func stringOf(t *testing.T, e observer.LoggedEntry, key string) string {
	t.Helper()

	for _, f := range e.Context {
		if f.Key == key && f.Type == zapcore.StringType {
			return f.String
		}
	}
	t.Fatalf("record %q has no string field %q: %+v", e.Message, key, e.Context)
	return ""
}

// A pass is one job: its record carries a job_id, the component, and how many
// reminders landed. That is what tells an idle loop from a dead one.
func TestRemindDueRecordsThePass(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1", "u2"}
	core, logs := observer.New(zapcore.DebugLevel)
	svc.logger = logger.NewWithCore(core)

	svc.RemindDue(context.Background(), repo.event.StartsAt.Add(-45*time.Minute))

	if len(n.sent) != 2 {
		t.Fatalf("want 2 reminders sent, got %d", len(n.sent))
	}

	e := entryWithEvent(t, logs, jobFinished)
	if e.Level != zapcore.InfoLevel {
		t.Errorf("a pass that sent reminders is Info, got %v", e.Level)
	}
	if got := intOf(t, e, "reminders"); got != int64(len(n.sent)) {
		t.Errorf("reminders = %d, want %d", got, len(n.sent))
	}
	if got := intOf(t, e, "failed"); got != 0 {
		t.Errorf("failed = %d, want 0", got)
	}
	if got := stringOf(t, e, "job_id"); got == "" {
		t.Error("the pass record must carry a job_id")
	}
	if got := stringOf(t, e, "job"); got != reminderJob {
		t.Errorf("job = %q, want %q", got, reminderJob)
	}
	if got := stringOf(t, e, "component"); got != "job" {
		t.Errorf("component = %q, want job", got)
	}
	if _, ok := durationOf(e); !ok {
		t.Error("the pass record must carry duration_ms")
	}
}

// A failed send is counted and recorded at Error, and the pass record still
// says how many landed: a partial pass is the one an operator must see.
func TestRemindDueRecordsSendFailures(t *testing.T) {
	svc, repo, n := attendService(t)
	repo.attendees = []string{"u1", "u2"}
	n.fails = 1
	core, logs := observer.New(zapcore.DebugLevel)
	svc.logger = logger.NewWithCore(core)

	svc.RemindDue(context.Background(), repo.event.StartsAt.Add(-45*time.Minute))

	fail := entryWithEvent(t, logs, sendFailed)
	if fail.Level != zapcore.ErrorLevel {
		t.Errorf("a failed send is Error, got %v", fail.Level)
	}
	if got := stringOf(t, fail, "job_id"); got == "" {
		t.Error("the failure record must carry the pass job_id")
	}

	pass := entryWithEvent(t, logs, jobFinished)
	if got := intOf(t, pass, "failed"); got != 1 {
		t.Errorf("failed = %d, want 1", got)
	}
	if got := intOf(t, pass, "reminders"); got != 1 {
		t.Errorf("reminders = %d, want the one that landed", got)
	}
}

// An idle pass runs every minute: it is a heartbeat at Debug, not 1440 Info
// records a day per instance.
func TestRemindDueIdlePassIsDebug(t *testing.T) {
	svc, repo, _ := attendService(t)
	core, logs := observer.New(zapcore.DebugLevel)
	svc.logger = logger.NewWithCore(core)

	// Two hours before the start is outside the reminder lead, so nothing is
	// due even though the repository lists the event.
	svc.RemindDue(context.Background(), repo.event.StartsAt.Add(-2*time.Hour))

	for _, e := range logs.All() {
		if e.Level == zapcore.InfoLevel || e.Level == zapcore.WarnLevel || e.Level == zapcore.ErrorLevel {
			t.Errorf("an idle pass must not log above Debug, got %v: %q", e.Level, e.Message)
		}
	}
	entryWithEvent(t, logs, jobFinished)
}

func durationOf(e observer.LoggedEntry) (int64, bool) {
	for _, f := range e.Context {
		if f.Key == "duration_ms" {
			return f.Integer, true
		}
	}
	return 0, false
}
