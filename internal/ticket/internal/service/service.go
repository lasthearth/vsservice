package service

import (
	"context"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/ticket/internal/ierror"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"go.uber.org/zap"
)

const (
	defaultPageSize = 20
	maxPageSize     = 50
)

func pageSize(n int32) int {
	if n <= 0 {
		return defaultPageSize
	}
	return min(int(n), maxPageSize)
}

func callerID(ctx context.Context) (string, error) {
	id, err := interceptor.GetUserID(ctx)
	if err != nil || id == "" {
		return "", pkgerr.Unauthenticated("missing user id")
	}
	return id, nil
}

// CreateTicket implements ticketv1.TicketServiceServer.
func (s *Service) CreateTicket(ctx context.Context, req *ticketv1.CreateTicketRequest) (*ticketv1.Ticket, error) {
	userID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	t, err := model.NewTicket(userID, req.GetSubject(), categoryFromProto(req.GetCategory()), req.GetText(), s.now())
	if err != nil {
		return nil, mapModelErr(err)
	}

	// A soft cap: the count and the insert are two round trips, so N parallel
	// creates can all pass it. Holding it exactly would need a transaction
	// around the pair, which the abuse it stops does not justify.
	active, err := s.repo.CountActiveByAuthor(ctx, userID)
	if err != nil {
		return nil, err
	}
	if active >= model.MaxActivePerAuthor {
		return nil, ierror.ErrActiveTicketLimit
	}

	created, err := s.repo.Create(ctx, t)
	if err != nil {
		return nil, err
	}

	s.notifier.TicketOpened(ctx, created)
	return toProto(created, model.ViewerAuthor), nil
}

// ListMyTickets implements ticketv1.TicketServiceServer.
func (s *Service) ListMyTickets(ctx context.Context, req *ticketv1.ListMyTicketsRequest) (*ticketv1.ListTicketsResponse, error) {
	userID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	tickets, next, err := s.repo.List(ctx, model.Query{
		Viewer:    model.ViewerAuthor,
		AuthorId:  userID,
		Limit:     pageSize(req.GetPageSize()),
		PageToken: req.GetPageToken(),
	})
	if err != nil {
		return nil, err
	}

	return &ticketv1.ListTicketsResponse{
		Tickets:       toProtos(tickets, model.ViewerAuthor),
		NextPageToken: next,
	}, nil
}

// GetTicket implements ticketv1.TicketServiceServer.
func (s *Service) GetTicket(ctx context.Context, req *ticketv1.GetTicketRequest) (*ticketv1.Ticket, error) {
	userID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	t, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if !t.IsAuthor(userID) {
		return nil, ierror.ErrNotFound
	}

	if t.AuthorUnread {
		// Failing to clear the mark is not worth failing the read.
		if err := s.repo.MarkRead(ctx, t.Id, false); err != nil {
			s.logger.WithMethod("GetTicket").Warn("failed to mark ticket read",
				zap.String("ticket_id", t.Id), zap.Error(err))
		} else {
			t.MarkReadByAuthor()
		}
	}

	return toProto(t, model.ViewerAuthor), nil
}

// PostTicketMessage implements ticketv1.TicketServiceServer.
func (s *Service) PostTicketMessage(ctx context.Context, req *ticketv1.PostTicketMessageRequest) (*ticketv1.Ticket, error) {
	userID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	now := s.now()
	updated, err := s.repo.Update(ctx, req.GetId(), func(_ context.Context, t *model.Ticket) (*model.Ticket, error) {
		if err := t.PostByAuthor(userID, req.GetText(), now); err != nil {
			return nil, mapModelErr(err)
		}
		// The author has just seen the whole thread.
		t.MarkReadByAuthor()
		return t, nil
	})
	if err != nil {
		return nil, err
	}

	// The stored text, not the request's: the model trimmed and validated it.
	s.notifier.AuthorWrote(ctx, updated, lastText(updated))
	return toProto(updated, model.ViewerAuthor), nil
}

// lastText is the newest message of a thread, empty when it has none.
func lastText(t *model.Ticket) string {
	if len(t.Messages) == 0 {
		return ""
	}
	return t.Messages[len(t.Messages)-1].Text
}

// CloseTicket implements ticketv1.TicketServiceServer.
func (s *Service) CloseTicket(ctx context.Context, req *ticketv1.CloseTicketRequest) (*ticketv1.Ticket, error) {
	userID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	now := s.now()
	updated, err := s.repo.Update(ctx, req.GetId(), func(_ context.Context, t *model.Ticket) (*model.Ticket, error) {
		if err := t.CloseByAuthor(userID, now); err != nil {
			return nil, mapModelErr(err)
		}
		return t, nil
	})
	if err != nil {
		return nil, err
	}

	return toProto(updated, model.ViewerAuthor), nil
}

// ReopenTicket implements ticketv1.TicketServiceServer.
func (s *Service) ReopenTicket(ctx context.Context, req *ticketv1.ReopenTicketRequest) (*ticketv1.Ticket, error) {
	userID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	now := s.now()
	updated, err := s.repo.Update(ctx, req.GetId(), func(ctx context.Context, t *model.Ticket) (*model.Ticket, error) {
		// Ownership and state first: a stranger's id or an already-open ticket
		// must not be answered with the caller's own limit.
		if err := t.ReopenByAuthor(userID, now); err != nil {
			return nil, mapModelErr(err)
		}
		active, err := s.repo.CountActiveByAuthor(ctx, userID)
		if err != nil {
			return nil, err
		}
		if active >= model.MaxActivePerAuthor {
			return nil, ierror.ErrActiveTicketLimit
		}
		return t, nil
	})
	if err != nil {
		return nil, err
	}

	s.notifier.AuthorWrote(ctx, updated, "")
	return toProto(updated, model.ViewerAuthor), nil
}

// GetMyTicketCounters implements ticketv1.TicketServiceServer.
func (s *Service) GetMyTicketCounters(ctx context.Context, _ *ticketv1.GetMyTicketCountersRequest) (*ticketv1.MyTicketCounters, error) {
	userID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	n, err := s.repo.CountUnreadByAuthor(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &ticketv1.MyTicketCounters{Unread: int32(n)}, nil
}
