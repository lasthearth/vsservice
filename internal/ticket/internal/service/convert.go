package service

import (
	"errors"
	"time"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	"github.com/lasthearth/vsservice/internal/ticket/internal/ierror"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var categories = map[model.Category]ticketv1.TicketCategory{
	model.CategoryBug:        ticketv1.TicketCategory_TICKET_CATEGORY_BUG,
	model.CategoryComplaint:  ticketv1.TicketCategory_TICKET_CATEGORY_COMPLAINT,
	model.CategoryDonate:     ticketv1.TicketCategory_TICKET_CATEGORY_DONATE,
	model.CategorySettlement: ticketv1.TicketCategory_TICKET_CATEGORY_SETTLEMENT,
	model.CategoryQuestion:   ticketv1.TicketCategory_TICKET_CATEGORY_QUESTION,
	model.CategoryOther:      ticketv1.TicketCategory_TICKET_CATEGORY_OTHER,
}

var statuses = map[model.Status]ticketv1.TicketStatus{
	model.StatusOpen:     ticketv1.TicketStatus_TICKET_STATUS_OPEN,
	model.StatusAnswered: ticketv1.TicketStatus_TICKET_STATUS_ANSWERED,
	model.StatusClosed:   ticketv1.TicketStatus_TICKET_STATUS_CLOSED,
}

// reverse finds the model value of a proto enum; zero when unset or unknown,
// which the model rejects or reads as "no filter".
func reverse[M comparable, P comparable](table map[M]P, p P) M {
	for m, v := range table {
		if v == p {
			return m
		}
	}
	var zero M
	return zero
}

func categoryFromProto(c ticketv1.TicketCategory) model.Category { return reverse(categories, c) }
func statusFromProto(s ticketv1.TicketStatus) model.Status       { return reverse(statuses, s) }

func ts(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func tsPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// toProto renders a ticket for one side. The author never sees internal notes,
// and each side gets its own unread mark and last activity.
func toProto(t *model.Ticket, viewer model.Viewer) *ticketv1.Ticket {
	out := &ticketv1.Ticket{
		Id:         t.Id,
		AuthorId:   t.AuthorId,
		Subject:    t.Subject,
		Category:   categories[t.Category],
		Status:     statuses[t.Status],
		AssigneeId: t.AssigneeId,
		CreatedAt:  ts(t.CreatedAt),
		ClosedAt:   tsPtr(t.ClosedAt),
	}

	messages := t.Messages
	if viewer == model.ViewerAuthor {
		messages = t.PublicMessages()
		out.Unread = t.AuthorUnread
		out.MessageCount = t.PublicCount
		out.LastMessageAt = ts(t.LastPublicAt)
	} else {
		out.Unread = t.StaffUnread
		out.MessageCount = t.MessageCount
		out.LastMessageAt = ts(t.LastMessageAt)
	}

	out.Messages = make([]*ticketv1.TicketMessage, 0, len(messages))
	for _, m := range messages {
		out.Messages = append(out.Messages, &ticketv1.TicketMessage{
			Id:        m.Id,
			AuthorId:  m.AuthorId,
			Staff:     m.Staff,
			Text:      m.Text,
			Internal:  m.Internal,
			CreatedAt: ts(m.CreatedAt),
		})
	}
	return out
}

func toProtos(list []model.Ticket, viewer model.Viewer) []*ticketv1.Ticket {
	out := make([]*ticketv1.Ticket, 0, len(list))
	for i := range list {
		p := toProto(&list[i], viewer)
		p.Messages = nil
		out = append(out, p)
	}
	return out
}

func replyToProto(r *model.SavedReply) *ticketv1.SavedReply {
	return &ticketv1.SavedReply{
		Id:        r.Id,
		Title:     r.Title,
		Text:      r.Text,
		UpdatedBy: r.UpdatedBy,
		UpdatedAt: ts(r.UpdatedAt),
	}
}

// mapModelErr turns a model refusal into the typed domain error the client
// sees; anything else passes through.
func mapModelErr(err error) error {
	switch {
	case errors.Is(err, model.ErrCategoryInvalid):
		return ierror.ErrCategoryInvalid
	case errors.Is(err, model.ErrStatusInvalid):
		return ierror.ErrStatusInvalid
	case errors.Is(err, model.ErrSubjectInvalid):
		return ierror.ErrSubjectInvalid
	case errors.Is(err, model.ErrTextInvalid):
		return ierror.ErrTextInvalid
	case errors.Is(err, model.ErrReplyTitleInvalid):
		return ierror.ErrReplyTitleInvalid
	case errors.Is(err, model.ErrClosed):
		return ierror.ErrClosed
	case errors.Is(err, model.ErrNotClosed):
		return ierror.ErrNotClosed
	case errors.Is(err, model.ErrThreadFull):
		return ierror.ErrThreadFull
	case errors.Is(err, model.ErrCloseWithNote):
		return ierror.ErrCloseWithNote
	case errors.Is(err, model.ErrAuthorRequired):
		return ierror.ErrAuthorRequired
	case errors.Is(err, model.ErrNotAuthor):
		// Someone else's ticket looks exactly like a missing one.
		return ierror.ErrNotFound
	default:
		return err
	}
}
