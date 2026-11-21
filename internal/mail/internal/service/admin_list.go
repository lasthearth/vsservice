package service

import (
	"context"
	"errors"

	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"go.uber.org/zap"
)

const (
	// adminMailDefaultPage is the page size when the caller asks for none: one
	// screen of rows.
	adminMailDefaultPage int64 = 20
	// adminMailMaxPage mirrors the ceiling the proto validation declares.
	adminMailMaxPage int64 = 100
)

// AdminListMail returns one page of sent mails, each with the number of players
// who claimed it. Admin.
func (s *Service) AdminListMail(ctx context.Context, req *mailv1.AdminListMailRequest) (*mailv1.AdminListMailResponse, error) {
	l := s.log.With(zap.String("method", "AdminListMail"))

	entries, next, err := s.repo.ListMailsAdmin(
		ctx,
		model.NewAdminMailFilter(req.GetRecipient(), req.GetBroadcastOnly()),
		req.GetPageToken(),
		adminPageSize(req.GetPageSize()),
	)
	if err != nil {
		// A page token the pagination package did not mint is a DomainError
		// carrying InvalidArgument; it must reach the client as 400, not be
		// rewritten to Internal.
		if _, ok := errors.AsType[*pkgerr.DomainError](err); ok {
			return nil, err
		}
		l.Error("failed to list mails", zap.Error(err))
		return nil, pkgerr.Internal("failed to list mails")
	}

	return &mailv1.AdminListMailResponse{
		Mails:         s.mapper.ToAdminMailsProto(entries),
		NextPageToken: next,
	}, nil
}

// adminPageSize clamps the requested page to the list's own bounds.
func adminPageSize(requested int32) int64 {
	switch {
	case requested <= 0:
		return adminMailDefaultPage
	case int64(requested) > adminMailMaxPage:
		return adminMailMaxPage
	default:
		return int64(requested)
	}
}
