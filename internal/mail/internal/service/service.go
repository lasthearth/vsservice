package service

import (
	"context"
	"errors"
	"time"

	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	"github.com/lasthearth/vsservice/internal/mail/internal/goverter"
	mailerr "github.com/lasthearth/vsservice/internal/mail/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

// ListMail returns the caller's mails joined with their derived per-player state.
func (s *Service) ListMail(ctx context.Context, _ *mailv1.ListMailRequest) (*mailv1.ListMailResponse, error) {
	l := s.log.With(zap.String("method", "ListMail"))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	mails, err := s.repo.ListMailsForRecipient(ctx, playerID)
	if err != nil {
		l.Error("failed to list mails", zap.Error(err))
		return nil, pkgerr.Internal("failed to list mails")
	}

	claims, err := s.repo.ListClaimsForPlayer(ctx, playerID)
	if err != nil {
		l.Error("failed to list claims", zap.Error(err))
		return nil, pkgerr.Internal("failed to list mails")
	}

	claimByMail := make(map[string]*model.MailClaim, len(claims))
	for _, c := range claims {
		claimByMail[c.MailID] = c
	}

	now := time.Now()
	pbs := s.mapper.ToMailsProto(mails)
	for i, m := range mails {
		pbs[i].State = goverter.MailStateToProto(deriveState(m, claimByMail[m.Id], now))
	}

	return &mailv1.ListMailResponse{Mails: pbs}, nil
}

// deriveState computes the per-player state from the mail document and the
// caller's claim row (nil = no row = UNREAD). Claimed is terminal and wins;
// otherwise revoked/expired are derived from the document.
func deriveState(m *model.Mail, claim *model.MailClaim, now time.Time) model.MailState {
	if claim != nil && claim.IsClaimed() {
		return model.MailStateClaimed
	}
	if m.Revoked {
		return model.MailStateRevoked
	}
	if m.IsExpiredAt(now) {
		return model.MailStateExpired
	}
	if claim == nil {
		return model.MailStateUnread
	}
	return claim.State
}

// MarkRead marks one of the caller's mails as read. Idempotent.
func (s *Service) MarkRead(ctx context.Context, req *mailv1.MarkReadRequest) (*mailv1.MarkReadResponse, error) {
	l := s.log.With(zap.String("method", "MarkRead"), zap.String("mail_id", req.GetMailId()))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	if _, err := s.ensureRecipient(ctx, req.GetMailId(), playerID); err != nil {
		return nil, err
	}

	if err := s.transitionClaim(ctx, req.GetMailId(), playerID, func(c *model.MailClaim) error {
		return c.MarkRead()
	}); err != nil {
		l.Error("failed to mark read", zap.Error(err))
		return nil, mapClaimErr(err)
	}

	return &mailv1.MarkReadResponse{}, nil
}

// Claim claims the attachments of one of the caller's mails. Idempotent. The
// claim row is written before physical delivery (VintageAPI grants by reading
// Mongo).
func (s *Service) Claim(ctx context.Context, req *mailv1.ClaimRequest) (*mailv1.ClaimResponse, error) {
	l := s.log.With(zap.String("method", "Claim"), zap.String("mail_id", req.GetMailId()))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	mail, err := s.ensureRecipient(ctx, req.GetMailId(), playerID)
	if err != nil {
		return nil, err
	}

	if err := s.claimMail(ctx, mail, playerID); err != nil {
		l.Error("failed to claim", zap.Error(err))
		return nil, mapClaimErr(err)
	}

	return &mailv1.ClaimResponse{}, nil
}

// ClaimAll claims every claimable mail for the caller.
func (s *Service) ClaimAll(ctx context.Context, _ *mailv1.ClaimAllRequest) (*mailv1.ClaimAllResponse, error) {
	l := s.log.With(zap.String("method", "ClaimAll"))

	playerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	mails, err := s.repo.ListMailsForRecipient(ctx, playerID)
	if err != nil {
		l.Error("failed to list mails", zap.Error(err))
		return nil, pkgerr.Internal("failed to claim mails")
	}

	claims, err := s.repo.ListClaimsForPlayer(ctx, playerID)
	if err != nil {
		l.Error("failed to list claims", zap.Error(err))
		return nil, pkgerr.Internal("failed to claim mails")
	}
	claimByMail := make(map[string]*model.MailClaim, len(claims))
	for _, c := range claims {
		claimByMail[c.MailID] = c
	}

	now := time.Now()
	claimed := make([]string, 0, len(mails))
	for _, m := range mails {
		if !m.HasAttachments() {
			continue
		}
		state := deriveState(m, claimByMail[m.Id], now)
		if state != model.MailStateUnread && state != model.MailStateRead {
			continue
		}
		if err := s.claimMail(ctx, m, playerID); err != nil {
			// Nothing-to-claim / not-claimable races are skipped; a hard error aborts.
			if errors.Is(err, model.ErrNothingToClaim) || errors.Is(err, mailerr.ErrNotClaimable) {
				continue
			}
			l.Error("failed to claim mail", zap.String("mail_id", m.Id), zap.Error(err))
			return nil, mapClaimErr(err)
		}
		claimed = append(claimed, m.Id)
	}

	return &mailv1.ClaimAllResponse{ClaimedMailIds: claimed}, nil
}

// ComposeMail creates a mail addressed to one player or to everyone. Admin.
func (s *Service) ComposeMail(ctx context.Context, req *mailv1.ComposeMailRequest) (*mailv1.ComposeMailResponse, error) {
	l := s.log.With(zap.String("method", "ComposeMail"))

	adminID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	recipient := req.GetUserId()
	if req.GetBroadcast() {
		recipient = model.RecipientBroadcast
	}
	if recipient == "" {
		return nil, pkgerr.InvalidArgument("recipient is required")
	}

	mail := model.NewMail(
		recipient,
		"admin:"+adminID,
		req.GetTitle(),
		req.GetBody(),
		goverter.AttachmentsProtoToModel(req.GetAttachments()),
		timestampToTimePtr(req),
		req.GetIdempotencyKey(),
	)

	created, err := s.repo.CreateMail(ctx, mail)
	if err != nil {
		l.Error("failed to create mail", zap.Error(err))
		return nil, pkgerr.Internal("failed to create mail")
	}

	return &mailv1.ComposeMailResponse{MailId: created.Id}, nil
}

// ComposeKitMail creates a mail whose attachments are a captured kit's contents,
// expanded server-side, addressed to one player or everyone (broadcast). Admin.
// Idempotent on idempotency_key. Fail-loud on a missing/empty kit.
func (s *Service) ComposeKitMail(ctx context.Context, req *mailv1.ComposeKitMailRequest) (*mailv1.ComposeKitMailResponse, error) {
	l := s.log.With(zap.String("method", "ComposeKitMail"), zap.String("kit_id", req.GetKitId()))

	adminID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, pkgerr.Unauthenticated(err.Error())
	}

	recipient := req.GetUserId()
	if req.GetBroadcast() {
		recipient = model.RecipientBroadcast
	}
	if recipient == "" {
		return nil, pkgerr.InvalidArgument("recipient is required")
	}

	// expandKit already normalizes a missing kit to ErrKitNotFound and an empty
	// kit to ErrKitEmpty; both are DomainErrors the interceptor maps to the
	// documented NOT_FOUND / FAILED_PRECONDITION statuses. Any other error is a
	// real failure.
	attachments, err := expandKit(ctx, s.kits, req.GetKitId())
	if err != nil {
		if isDomainError(err, mailerr.ErrKitEmpty) || isDomainError(err, mailerr.ErrKitNotFound) {
			return nil, err
		}
		l.Error("failed to expand kit", zap.Error(err))
		return nil, pkgerr.Internal("failed to expand kit")
	}

	mail := model.NewMail(
		recipient,
		"admin:"+adminID,
		req.GetTitle(),
		req.GetBody(),
		attachments,
		kitTimestampToTimePtr(req),
		req.GetIdempotencyKey(),
	)

	created, err := s.repo.CreateMail(ctx, mail)
	if err != nil {
		l.Error("failed to create kit mail", zap.Error(err))
		return nil, pkgerr.Internal("failed to create mail")
	}

	return &mailv1.ComposeKitMailResponse{MailId: created.Id}, nil
}

// RevokeMail sets the revoked flag on a mail document. Admin. Idempotent.
func (s *Service) RevokeMail(ctx context.Context, req *mailv1.RevokeMailRequest) (*mailv1.RevokeMailResponse, error) {
	l := s.log.With(zap.String("method", "RevokeMail"), zap.String("mail_id", req.GetMailId()))

	_, err := s.repo.UpdateMail(ctx, req.GetMailId(), func(_ context.Context, m *model.Mail) (*model.Mail, error) {
		m.Revoke()
		return m, nil
	})
	if err != nil {
		if isDomainError(err, mailerr.ErrNotFound) {
			return nil, mailerr.ErrNotFound
		}
		l.Error("failed to revoke mail", zap.Error(err))
		return nil, pkgerr.Internal("failed to revoke mail")
	}

	return &mailv1.RevokeMailResponse{}, nil
}

// ensureRecipient loads the mail and verifies the caller is allowed to see it
// (targeted at them or a broadcast). Returns ierror.ErrNotFound otherwise, so a
// mail addressed to someone else is indistinguishable from a missing one.
func (s *Service) ensureRecipient(ctx context.Context, mailID, playerID string) (*model.Mail, error) {
	mail, err := s.repo.GetMail(ctx, mailID)
	if err != nil {
		return nil, mailerr.ErrNotFound
	}
	if mail.Recipient != model.RecipientBroadcast && mail.Recipient != playerID {
		return nil, mailerr.ErrNotFound
	}
	return mail, nil
}

// transitionClaim applies fn to the caller's claim row, materializing the row
// lazily when none exists yet. The fresh row already reflects the transition
// (read/claimed) so the first insert is the claimed-before-grant write.
func (s *Service) transitionClaim(ctx context.Context, mailID, playerID string, fn func(*model.MailClaim) error) error {
	fresh := model.NewMailClaim(mailID, playerID)
	if err := fn(fresh); err != nil {
		return err
	}

	created, err := s.repo.InsertClaim(ctx, fresh)
	if err != nil {
		return err
	}
	if created {
		return nil
	}
	// A row already existed: re-apply the transition on the stored state.
	_, err = s.repo.UpdateClaim(ctx, mailID, playerID, func(_ context.Context, c *model.MailClaim) (*model.MailClaim, error) {
		if err := fn(c); err != nil {
			return nil, err
		}
		return c, nil
	})
	return err
}

// claimMail runs the claimed-before-grant write for one mail: guard on the mail
// document (revoked/expired/no-attachments), then transition the claim row.
func (s *Service) claimMail(ctx context.Context, mail *model.Mail, playerID string) error {
	if !mail.HasAttachments() {
		return model.ErrNothingToClaim
	}
	if mail.Revoked || mail.IsExpiredAt(time.Now()) {
		return mailerr.ErrNotClaimable
	}
	return s.transitionClaim(ctx, mail.Id, playerID, func(c *model.MailClaim) error {
		return c.Claim(true)
	})
}

func timestampToTimePtr(req *mailv1.ComposeMailRequest) *time.Time {
	ts := req.GetExpiresAt()
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func kitTimestampToTimePtr(req *mailv1.ComposeKitMailRequest) *time.Time {
	ts := req.GetExpiresAt()
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

// isDomainError reports whether err is the given sentinel domain error.
func isDomainError(err, sentinel error) bool {
	return errors.Is(err, sentinel)
}

// mapClaimErr maps model/domain claim errors to typed domain errors the
// interceptor turns into gRPC statuses.
func mapClaimErr(err error) error {
	switch {
	case errors.Is(err, model.ErrNothingToClaim):
		return mailerr.ErrNothingToClaim
	case errors.Is(err, mailerr.ErrNotClaimable):
		return mailerr.ErrNotClaimable
	case errors.Is(err, model.ErrMailClaimTerminal):
		return mailerr.ErrNotClaimable
	case errors.Is(err, mailerr.ErrNotFound):
		return mailerr.ErrNotFound
	default:
		return pkgerr.Internal("failed to update mail state")
	}
}
