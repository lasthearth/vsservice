package service

import (
	"context"
	"fmt"
	"time"

	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

// ReminderInterval is how often the reminder loop looks for events about to
// start.
const ReminderInterval = time.Minute

// remindTimeout bounds one pass. Without it a single hung Mongo call stalls the
// loop while the reminder window slides forward, and an event whose start
// passes in the meantime is never reminded again — with no error anywhere.
const remindTimeout = 15 * time.Second

// reminderJob names the loop in its records, and jobFinished/jobFailed are the
// stable keys of one pass. A pass runs every minute, so its records are the
// only way to tell "nothing was due" from "the loop stopped running".
const (
	reminderJob = "event.reminders"
	jobFinished = "job.run.finished"
	jobFailed   = "job.run.failed"

	sendFailed    = "event.reminder.send_failed"
	claimFailed   = "event.reminder.claim_failed"
	unclaimFailed = "event.reminder.unclaim_failed"
	listFailed    = "event.reminder.list_failed"
)

// RemindDue reminds attendees of every event that starts within
// model.ReminderLead of now. Each attendee is reminded once per start time,
// even with several service instances (see Repository.ClaimReminders).
func (s *Service) RemindDue(ctx context.Context, now time.Time) {
	if s.cnuc == nil {
		return
	}

	// One pass is one job: the loop has no request to inherit a correlation id
	// from, so it mints one and every record of this pass carries it.
	ctx, _ = logger.WithJob(ctx, reminderJob)
	log := logger.From(ctx, s.logger)

	ctx, cancel := context.WithTimeout(ctx, remindTimeout)
	defer cancel()

	start := time.Now()

	events, err := s.repo.ListStartingWithin(ctx, now, now.Add(model.ReminderLead))
	if err != nil {
		log.Error("failed to list events to remind about",
			zap.String("event", listFailed),
			zap.Error(err))
		return
	}

	sent, failed, claimsFailed := 0, 0, 0
	for i := range events {
		e := &events[i]
		if !e.ReminderDue(now) {
			continue
		}

		ids, err := s.repo.ClaimReminders(ctx, e.Id, e.StartsAt)
		if err != nil {
			// The attendee count is unknown here, so a failed claim is counted
			// on its own instead of inflating the reminder total.
			claimsFailed++
			log.Error("failed to claim reminders",
				zap.String("event", claimFailed),
				zap.String("event_id", e.Id),
				zap.Error(err))
			continue
		}

		text := reminderText(e, now)
		for _, id := range ids {
			if err := s.cnuc.CreateNotification(ctx, "Скоро событие", text, notificationuc.WithUserId(id)); err != nil {
				failed++
				log.Error("failed to send reminder",
					zap.String("event", sendFailed),
					zap.String("event_id", e.Id),
					zap.Error(err))
				// The claim is given back, so the next tick retries instead of
				// silently costing this player their only reminder. This is the
				// case a rolling deploy hits: the lifecycle context is cancelled
				// mid-loop, every remaining send fails, and without the give-back
				// the claims would outlive the notifications.
				if uerr := s.repo.UnclaimReminder(ctx, e.Id, id, e.StartsAt); uerr != nil {
					log.Error("failed to give the reminder claim back",
						zap.String("event", unclaimFailed),
						zap.String("event_id", e.Id),
						zap.Error(uerr))
				}
				continue
			}
			sent++
		}
	}

	fields := []zap.Field{
		zap.Int("events", len(events)),
		zap.Int("reminders", sent),
		zap.Int("failed", failed),
		zap.Int("claims_failed", claimsFailed),
		zap.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if sent == 0 && failed == 0 && claimsFailed == 0 {
		// An idle pass is a heartbeat, not news: at Info it would be 1440
		// records a day per instance saying nothing happened.
		log.Debug("reminder pass idle", append(fields, zap.String("event", jobFinished))...)
		return
	}
	log.Info("reminder pass finished", append(fields, zap.String("event", jobFinished))...)
}

// RunReminders calls RemindDue every ReminderInterval until ctx is done.
func (s *Service) RunReminders(ctx context.Context) {
	ticker := time.NewTicker(ReminderInterval)
	defer ticker.Stop()

	for {
		s.RemindDue(ctx, s.now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// reminderText is "«Ярмарка» начнётся через 45 мин. Место: у моста".
func reminderText(e *model.Event, now time.Time) string {
	minutes := int(e.StartsAt.Sub(now).Round(time.Minute) / time.Minute)
	text := fmt.Sprintf("«%s» начнётся через %d мин", e.Title, max(minutes, 1))
	if e.Location != "" {
		text += ". Место: " + e.Location
	}
	return text
}
