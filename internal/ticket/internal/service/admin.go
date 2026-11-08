package service

import (
	"context"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	"github.com/lasthearth/vsservice/internal/ticket/internal/ierror"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

var assigneeScopes = map[ticketv1.AssigneeFilter]model.AssigneeScope{
	ticketv1.AssigneeFilter_ASSIGNEE_FILTER_UNSPECIFIED: model.AssigneeAny,
	ticketv1.AssigneeFilter_ASSIGNEE_FILTER_MINE:        model.AssigneeMine,
	ticketv1.AssigneeFilter_ASSIGNEE_FILTER_NOBODY:      model.AssigneeNobody,
}

// AdminListTickets implements ticketv1.TicketServiceServer.
func (s *Service) AdminListTickets(ctx context.Context, req *ticketv1.AdminListTicketsRequest) (*ticketv1.ListTicketsResponse, error) {
	staffID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	tickets, next, err := s.repo.List(ctx, model.Query{
		Viewer:    model.ViewerStaff,
		AuthorId:  req.GetAuthorId(),
		Status:    statusFromProto(req.GetStatus()),
		Category:  categoryFromProto(req.GetCategory()),
		Assignee:  assigneeScopes[req.GetAssignee()],
		StaffId:   staffID,
		Limit:     pageSize(req.GetPageSize()),
		PageToken: req.GetPageToken(),
	})
	if err != nil {
		return nil, err
	}

	return &ticketv1.ListTicketsResponse{
		Tickets:       toProtos(tickets, model.ViewerStaff),
		NextPageToken: next,
	}, nil
}

// AdminGetTicket implements ticketv1.TicketServiceServer.
func (s *Service) AdminGetTicket(ctx context.Context, req *ticketv1.AdminGetTicketRequest) (*ticketv1.Ticket, error) {
	t, err := s.repo.Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if t.StaffUnread {
		// Failing to clear the mark is not worth failing the read.
		if err := s.repo.MarkRead(ctx, t.Id, true); err != nil {
			s.logger.WithMethod("AdminGetTicket").Warn("failed to mark ticket read",
				zap.String("ticket_id", t.Id), zap.Error(err))
		} else {
			t.MarkReadByStaff()
		}
	}

	return toProto(t, model.ViewerStaff), nil
}

// AdminReplyTicket implements ticketv1.TicketServiceServer.
func (s *Service) AdminReplyTicket(ctx context.Context, req *ticketv1.AdminReplyTicketRequest) (*ticketv1.Ticket, error) {
	staffID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	now := s.now()
	updated, err := s.repo.Update(ctx, req.GetId(), func(_ context.Context, t *model.Ticket) (*model.Ticket, error) {
		if err := t.ReplyByStaff(staffID, req.GetText(), req.GetInternal(), req.GetClose(), now); err != nil {
			return nil, mapModelErr(err)
		}
		return t, nil
	})
	if err != nil {
		return nil, err
	}

	if !req.GetInternal() {
		s.notifier.StaffReplied(ctx, updated)
	}
	return toProto(updated, model.ViewerStaff), nil
}

// AdminUpdateTicket implements ticketv1.TicketServiceServer.
func (s *Service) AdminUpdateTicket(ctx context.Context, req *ticketv1.AdminUpdateTicketRequest) (*ticketv1.Ticket, error) {
	now := s.now()
	updated, err := s.repo.Update(ctx, req.GetId(), func(_ context.Context, t *model.Ticket) (*model.Ticket, error) {
		if req.Category != nil {
			if err := t.SetCategory(categoryFromProto(req.GetCategory())); err != nil {
				return nil, mapModelErr(err)
			}
		}
		if req.Status != nil {
			if err := t.SetStatus(statusFromProto(req.GetStatus()), now); err != nil {
				return nil, mapModelErr(err)
			}
		}
		if req.AssigneeId != nil {
			t.Assign(req.GetAssigneeId())
		}
		return t, nil
	})
	if err != nil {
		return nil, err
	}

	return toProto(updated, model.ViewerStaff), nil
}

// AdminGetTicketCounters implements ticketv1.TicketServiceServer.
func (s *Service) AdminGetTicketCounters(ctx context.Context, _ *ticketv1.AdminGetTicketCountersRequest) (*ticketv1.AdminTicketCounters, error) {
	staffID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	c, err := s.repo.StaffCounters(ctx, staffID)
	if err != nil {
		return nil, err
	}
	return &ticketv1.AdminTicketCounters{
		Open:           int32(c.Open),
		OpenUnassigned: int32(c.OpenUnassigned),
		OpenMine:       int32(c.OpenMine),
		Unread:         int32(c.Unread),
	}, nil
}

// ListSavedReplies implements ticketv1.TicketServiceServer.
func (s *Service) ListSavedReplies(ctx context.Context, _ *ticketv1.ListSavedRepliesRequest) (*ticketv1.ListSavedRepliesResponse, error) {
	replies, err := s.repo.ListReplies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*ticketv1.SavedReply, 0, len(replies))
	for i := range replies {
		out = append(out, replyToProto(&replies[i]))
	}
	return &ticketv1.ListSavedRepliesResponse{Replies: out}, nil
}

// CreateSavedReply implements ticketv1.TicketServiceServer.
func (s *Service) CreateSavedReply(ctx context.Context, req *ticketv1.CreateSavedReplyRequest) (*ticketv1.SavedReply, error) {
	staffID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	r, err := model.NewSavedReply(req.GetTitle(), req.GetText(), staffID, s.now())
	if err != nil {
		return nil, mapModelErr(err)
	}

	n, err := s.repo.CountReplies(ctx)
	if err != nil {
		return nil, err
	}
	if n >= model.MaxSavedReplies {
		return nil, ierror.ErrSavedReplyLimit
	}

	created, err := s.repo.CreateReply(ctx, r)
	if err != nil {
		return nil, err
	}
	return replyToProto(created), nil
}

// UpdateSavedReply implements ticketv1.TicketServiceServer.
func (s *Service) UpdateSavedReply(ctx context.Context, req *ticketv1.UpdateSavedReplyRequest) (*ticketv1.SavedReply, error) {
	staffID, err := callerID(ctx)
	if err != nil {
		return nil, err
	}

	now := s.now()
	updated, err := s.repo.UpdateReply(ctx, req.GetId(), func(_ context.Context, r *model.SavedReply) (*model.SavedReply, error) {
		if err := r.Edit(req.GetTitle(), req.GetText(), staffID, now); err != nil {
			return nil, mapModelErr(err)
		}
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	return replyToProto(updated), nil
}

// DeleteSavedReply implements ticketv1.TicketServiceServer.
func (s *Service) DeleteSavedReply(ctx context.Context, req *ticketv1.DeleteSavedReplyRequest) (*emptypb.Empty, error) {
	if err := s.repo.DeleteReply(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
