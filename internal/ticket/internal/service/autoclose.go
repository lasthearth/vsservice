package service

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"go.uber.org/zap"
)

const (
	autoCloseJob      = "ticket_autoclose"
	autoCloseInterval = time.Hour
	autoCloseBatch    = 200
	autoCloseTimeout  = 2 * time.Minute
)

// CloseStale closes the ANSWERED tickets the players left quiet for
// model.AutoCloseAfter.
func (s *Service) CloseStale(ctx context.Context, now time.Time) {
	ctx, _ = logger.WithJob(ctx, autoCloseJob)
	log := logger.From(ctx, s.logger)

	ctx, cancel := context.WithTimeout(ctx, autoCloseTimeout)
	defer cancel()

	ids, err := s.repo.ListAutoCloseDue(ctx, now.Add(-model.AutoCloseAfter), autoCloseBatch)
	if err != nil {
		log.Error("failed to list stale tickets", zap.Error(err))
		return
	}

	closed := 0
	for _, id := range ids {
		_, err := s.repo.Update(ctx, id, func(_ context.Context, t *model.Ticket) (*model.Ticket, error) {
			// Re-checked on the fresh document: the player may have answered
			// since the list was read.
			t.AutoClose(now)
			return t, nil
		})
		if err != nil {
			log.Warn("failed to close stale ticket", zap.String("ticket_id", id), zap.Error(err))
			continue
		}
		closed++
	}
	if closed > 0 {
		log.Info("closed stale tickets", zap.Int("count", closed))
	}
}

// RunAutoClose calls CloseStale every hour until ctx is done.
func (s *Service) RunAutoClose(ctx context.Context) {
	ticker := time.NewTicker(autoCloseInterval)
	defer ticker.Stop()

	for {
		s.CloseStale(ctx, s.now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
