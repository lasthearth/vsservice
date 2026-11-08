package service

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"github.com/lasthearth/vsservice/internal/ticket/internal/repository"
)

var _ Repository = (*repository.Repository)(nil)

// Repository stores tickets and saved replies.
type Repository interface {
	Create(ctx context.Context, t *model.Ticket) (*model.Ticket, error)
	Get(ctx context.Context, id string) (*model.Ticket, error)
	Update(
		ctx context.Context,
		id string,
		fn func(ctx context.Context, t *model.Ticket) (*model.Ticket, error),
	) (*model.Ticket, error)
	List(ctx context.Context, q model.Query) ([]model.Ticket, string, error)
	CountActiveByAuthor(ctx context.Context, authorId string) (int64, error)
	CountUnreadByAuthor(ctx context.Context, authorId string) (int64, error)
	StaffCounters(ctx context.Context, staffId string) (model.StaffCounters, error)
	ListAutoCloseDue(ctx context.Context, cutoff time.Time, limit int) ([]string, error)

	ListReplies(ctx context.Context) ([]model.SavedReply, error)
	CountReplies(ctx context.Context) (int64, error)
	CreateReply(ctx context.Context, r *model.SavedReply) (*model.SavedReply, error)
	UpdateReply(
		ctx context.Context,
		id string,
		fn func(ctx context.Context, r *model.SavedReply) (*model.SavedReply, error),
	) (*model.SavedReply, error)
	DeleteReply(ctx context.Context, id string) error
}

// Notifier tells the other side of a ticket that something happened. It is
// called after the change is stored and must not fail the request: delivery
// problems are its own to log.
type Notifier interface {
	// TicketOpened tells the staff about a new ticket.
	TicketOpened(ctx context.Context, t *model.Ticket)
	// AuthorWrote tells the staff the author answered or reopened the ticket.
	AuthorWrote(ctx context.Context, t *model.Ticket, text string)
	// StaffReplied tells the author the staff answered.
	StaffReplied(ctx context.Context, t *model.Ticket)
}
