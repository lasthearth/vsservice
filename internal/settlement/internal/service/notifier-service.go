package service

import (
	"context"
	"errors"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
)

// UserNotifier sends a site notification to one user.
type UserNotifier interface {
	NotifyUser(ctx context.Context, userId, title, message string) error
}

// createNotifier adapts the notification use case to UserNotifier.
type createNotifier struct {
	uc *notificationuc.Create
}

func (n createNotifier) NotifyUser(ctx context.Context, userId, title, message string) error {
	return n.uc.CreateNotification(ctx, title, message, notificationuc.WithUserId(userId))
}

// settlementProto maps a settlement and adds what lives outside its document:
// the position of its notifier block. A failing read leaves the position unset
// rather than failing the whole read.
func (s *Service) settlementProto(ctx context.Context, set model.Settlement) *settlementv1.Settlement {
	out := s.mapper.ToSettlementProto(set)

	placement, found, err := s.dbRepo.GetNotifier(ctx, set.Id)
	if err != nil {
		s.log.Warn("failed to read settlement notifier",
			zap.Error(err), zap.String("settlement_id", set.Id))
		return out
	}
	if found {
		out.NotifierPosition = s.mapper.ToVector3Proto(placement.Position)
	}
	return out
}

// deliverNotifier mails the new settlement's leader the notifier block of the
// settlement's type. It runs inside the approval transaction: ctx carries its
// session, so the mail commits or rolls back with the settlement. The key
// makes a retried transaction hand out one block only.
func (s *Service) deliverNotifier(ctx context.Context, set model.Settlement) error {
	return s.sendNotifierMail(ctx, set, set.Leader.UserId, model.NotifierDeliveryKey(set.Id),
		"Глашатай вашего поселения",
		"Поселение «"+set.Name+"» одобрено. В этом письме блок глашатая ("+set.Type.Title()+
			"): поставьте его на земле поселения, и оно появится на карте.")
}

// sendNotifierMail composes the mail that carries the notifier block of the
// settlement's current type.
func (s *Service) sendNotifierMail(ctx context.Context, set model.Settlement, userId, key, title, body string) error {
	code, ok := set.Type.NotifierBlockCode()
	if !ok {
		return ierror.ErrNotifierUnavailable
	}
	if userId == "" {
		return ierror.ErrNoOwnerToDeliver
	}
	return s.mail.ComposeSystemItemMail(ctx, mailcompose.SenderSettlement, userId, title, body, key,
		[]mailcompose.ItemSpec{{
			GameCode: code,
			Quantity: 1,
			Type:     model.NotifierBlockKind,
		}})
}

// upgraded reports whether the approval raised the settlement's type.
func upgraded(res ApprovalResult) bool {
	return !res.Created && res.PreviousType != "" && res.PreviousType != res.Settlement.Type
}

// mailOwnersAboutUpgrade sends the owners, and only them, a mail without
// attachments about the new level. The block itself is exchanged by the game
// server. Runs inside the approval transaction.
func (s *Service) mailOwnersAboutUpgrade(ctx context.Context, res ApprovalResult) error {
	if !upgraded(res) {
		return nil
	}
	set := res.Settlement
	for _, owner := range set.OwnerIds() {
		if err := s.mail.ComposeNotificationMail(ctx, mailcompose.SenderSettlement, owner,
			"Поселение повышено",
			"Поселение «"+set.Name+"» повышено: "+set.Type.Title()+
				". Глашатай на земле поселения обновится сам.",
			model.UpgradeNoticeKey(set.Id, set.Type, res.RequestedAt, owner),
		); err != nil {
			return err
		}
	}
	return nil
}

// notifyMembersAboutUpgrade tells every member, owners included, on the site.
// The approval has been committed, so a failing notifier must not undo it: the
// failure is only logged.
func (s *Service) notifyMembersAboutUpgrade(ctx context.Context, res ApprovalResult) {
	if s.notices == nil || !upgraded(res) {
		return
	}
	set := res.Settlement
	l := s.log.WithMethod("Approve").With(zap.String("settlement_id", set.Id))
	for _, id := range set.MemberIds() {
		if err := s.notices.NotifyUser(ctx,
			id,
			"Поселение повышено",
			"Поселение «"+set.Name+"» повышено: "+set.Type.Title()+".",
		); err != nil {
			l.Warn("failed to send upgrade notification", zap.Error(err), zap.String("user_id", id))
		}
	}
}

// ReissueNotifier implements settlementv1.SettlementServiceServer.
func (s *Service) ReissueNotifier(ctx context.Context, req *settlementv1.ReissueNotifierRequest) (*settlementv1.ReissueNotifierResponse, error) {
	l := s.log.WithMethod("ReissueNotifier").With(zap.String("settlement_id", req.GetSettlementId()))

	set, err := s.dbRepo.GetSettlement(ctx, req.GetSettlementId())
	if err != nil {
		return nil, err
	}

	recipient := req.GetUserId()
	if recipient == "" {
		recipient = set.Leader.UserId
	} else if !set.IsOwner(recipient) {
		return nil, ierror.ErrNoOwnerToDeliver
	}

	var number int
	err = s.dbRepo.InTransaction(ctx, func(ctx context.Context) error {
		// The counter and the mail commit together, so a number is never spent
		// without its mail and a mail never goes out under a reused number.
		updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
			func(_ context.Context, cur *model.Settlement) (*model.Settlement, error) {
				number = cur.NextNotifierReissue()
				return cur, nil
			})
		if err != nil {
			return err
		}

		return s.sendNotifierMail(ctx, *updated, recipient,
			model.NotifierReissueKey(updated.Id, number),
			"Глашатай вашего поселения",
			"Администрация повторно выдала блок глашатая поселения «"+updated.Name+"» ("+updated.Type.Title()+").")
	})
	if err != nil {
		if !errors.Is(err, ierror.ErrNotifierUnavailable) && !errors.Is(err, ierror.ErrNoOwnerToDeliver) {
			l.Error("failed to reissue notifier", zap.Error(err))
		}
		return nil, err
	}

	l.Info("notifier reissued", zap.Int("number", number), zap.String("user_id", recipient))
	return &settlementv1.ReissueNotifierResponse{Number: int32(number)}, nil
}
