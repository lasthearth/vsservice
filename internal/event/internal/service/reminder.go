package service

import (
	"context"
	"fmt"
	"time"

	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"go.uber.org/zap"
)

// ReminderInterval is how often the reminder loop looks for events about to
// start.
const ReminderInterval = time.Minute

// remindTimeout bounds one pass. Without it a single hung Mongo call stalls the
// loop while the reminder window slides forward, and an event whose start
// passes in the meantime is never reminded again — with no error anywhere.
const remindTimeout = 15 * time.Second

// RemindDue reminds attendees of every event that starts within
// model.ReminderLead of now. Each attendee is reminded once per start time,
// even with several service instances (see Repository.ClaimReminders).
func (s *Service) RemindDue(ctx context.Context, now time.Time) {
	if s.cnuc == nil {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, remindTimeout)
	defer cancel()

	events, err := s.repo.ListStartingWithin(ctx, now, now.Add(model.ReminderLead))
	if err != nil {
		s.logger.Error("failed to list events to remind about", zap.Error(err))
		return
	}

	for i := range events {
		e := &events[i]
		if !e.ReminderDue(now) {
			continue
		}

		ids, err := s.repo.ClaimReminders(ctx, e.Id, e.StartsAt)
		if err != nil {
			s.logger.Error("failed to claim reminders", zap.String("event_id", e.Id), zap.Error(err))
			continue
		}

		text := reminderText(e, now)
		for _, id := range ids {
			if err := s.cnuc.CreateNotification(ctx, "Скоро событие", text, notificationuc.WithUserId(id)); err != nil {
				s.logger.Error("failed to send reminder", zap.String("event_id", e.Id), zap.Error(err))
				// The claim is given back, so the next tick retries instead of
				// silently costing this player their only reminder. This is the
				// case a rolling deploy hits: the lifecycle context is cancelled
				// mid-loop, every remaining send fails, and without the give-back
				// the claims would outlive the notifications.
				if uerr := s.repo.UnclaimReminder(ctx, e.Id, id, e.StartsAt); uerr != nil {
					s.logger.Error("failed to give the reminder claim back", zap.String("event_id", e.Id), zap.Error(uerr))
				}
			}
		}
	}
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
