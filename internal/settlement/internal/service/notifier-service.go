package service

import (
	"context"
	"errors"
	"time"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
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

// readNotifier is the one place that decides what a failing notifier read means,
// so Submit and every response that carries a Settlement agree on it.
//
// found=false is a normal answer: no block stands, and notifier_position stays
// unset. Anything else fails closed with ErrNotifierRead. Serving a position that
// merely looks unset would make the site render the upgrade form as editable and
// let Submit drop the coordinates the player typed.
func (s *Service) readNotifier(ctx context.Context, settlementID string) (model.NotifierPlacement, bool, error) {
	placement, found, err := s.dbRepo.GetNotifier(ctx, settlementID)
	if err != nil {
		s.log.Error("failed to read settlement notifier",
			zap.Error(err), zap.String("settlement_id", settlementID))
		return model.NotifierPlacement{}, false, ierror.ErrNotifierRead
	}
	return placement, found, nil
}

// settlementProto maps a settlement and adds what lives outside its document: the
// position of its notifier block. Every rpc that returns a single settlement goes
// through it, so the field the proto documents is filled wherever one settlement
// is served, and not only by Get and GetByUserId.
func (s *Service) settlementProto(ctx context.Context, set model.Settlement) (*settlementv1.Settlement, error) {
	out := s.mapper.ToSettlementProto(set)

	// A guild answers no position and pays no read for one: its herald carries no settlement binding
	// and never writes a placement, so there is nothing this field could be filled from. Every rpc
	// that serves a single group comes through here, so all of them agree on it.
	if set.IsGuild() {
		return out, nil
	}

	placement, found, err := s.readNotifier(ctx, set.Id)
	if err != nil {
		return nil, err
	}
	if found {
		out.NotifierPosition = s.mapper.ToVector3Proto(placement.Position)
	}
	return out, nil
}

// deliverNotifier mails the new group's leader the notifier block of its type: the block of the
// settlement tier, or the guild herald for a guild. It runs inside the approval transaction: ctx
// carries its session, so the mail commits or rolls back with the settlement. The key names the
// submission, so a retried transaction hands out one block only while a settlement re-created under
// the same id gets its own.
func (s *Service) deliverNotifier(ctx context.Context, res ApprovalResult) error {
	set := res.Settlement
	title, body := notifierMailText(set)
	return s.sendNotifierMail(ctx, set, set.Leader.UserId,
		model.NotifierDeliveryKey(set.Id, res.RequestedAt), title, body)
}

// notifierMailText is the player-facing text of the mail that carries a group's notifier block. The
// two kinds word it apart: a settlement block puts the settlement on the map, while a guild herald
// carries no settlement binding and only marks the ground the guild meets on.
func notifierMailText(set model.Settlement) (title, body string) {
	if set.IsGuild() {
		return "Глашатай вашей гильдии",
			"Гильдия «" + set.Name + "» одобрена. В этом письме блок глашатая гильдий: поставьте его" +
				" там, где собирается гильдия, и игроки рядом увидят её название."
	}
	return "Глашатай вашего поселения",
		"Поселение «" + set.Name + "» одобрено. В этом письме блок глашатая (" + set.Type.Title() +
			"): поставьте его на земле поселения, и оно появится на карте."
}

// reissueMailText is the player-facing text of the mail that carries a notifier block an admin sent
// again.
func reissueMailText(set model.Settlement) (title, body string) {
	if set.IsGuild() {
		return "Глашатай вашей гильдии",
			"Администрация повторно выдала блок глашатая гильдий «" + set.Name + "»."
	}
	return "Глашатай вашего поселения",
		"Администрация повторно выдала блок глашатая поселения «" + set.Name + "» (" +
			set.Type.Title() + ")."
}

// joinNoun is the Russian accusative a player-facing text uses for the group a player joined:
// "в поселение" for a settlement, "в гильдию" for a guild. These texts are assembled on the server,
// so the word has to agree with the group it names.
func joinNoun(kind model.SettlementKind) string {
	if kind == model.KindGuild {
		return "гильдию"
	}
	return "поселение"
}

// newcomerNoun is the Russian word for the player who joined: a settlement takes a "житель", a guild
// an "участник".
func newcomerNoun(kind model.SettlementKind) string {
	if kind == model.KindGuild {
		return "участник"
	}
	return "житель"
}

// newcomerTitle is the notification title that announces one newcomer.
func newcomerTitle(kind model.SettlementKind) string {
	return "Новый " + newcomerNoun(kind)
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
		// Same guard sendNotifierMail has: an owner without a user id has no
		// mailbox, and the approval must say so instead of composing a mail that
		// nobody can ever read.
		if owner == "" {
			return ierror.ErrNoOwnerToDeliver
		}
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

// notifyWorkers bounds how many site notifications go out at once. The members of
// a large settlement must not turn into one sequential database write each on the
// Approve response path, where the admin is waiting for the answer.
const notifyWorkers = 8

// notifyMembersAboutUpgrade tells every member, owners included, on the site.
// The approval has been committed, so a failing notifier must not undo it: the
// failure is only logged. The group bounds concurrency and nothing else — no task
// returns an error, so it never cancels the rest and never changes the response.
func (s *Service) notifyMembersAboutUpgrade(ctx context.Context, res ApprovalResult) {
	if s.notices == nil || !upgraded(res) {
		return
	}
	set := res.Settlement
	l := s.log.WithMethod("Approve").With(zap.String("settlement_id", set.Id))
	title := "Поселение повышено"
	body := "Поселение «" + set.Name + "» повышено: " + set.Type.Title() + "."

	var g errgroup.Group
	g.SetLimit(notifyWorkers)
	for _, id := range set.MemberIds() {
		g.Go(func() error {
			if err := s.notices.NotifyUser(ctx, id, title, body); err != nil {
				l.Warn("failed to send upgrade notification", zap.Error(err), zap.String("user_id", id))
			}
			return nil
		})
	}
	// Every task returns nil, so Wait returns nil; it is only the barrier that
	// keeps the best-effort notifications from outliving the request.
	_ = g.Wait()
}

// submissionStamp returns the timestamp that tells one life of a settlement id
// apart from the next. The settlement id is the request id, and DeleteSettlement
// removes the settlement but never the request, so a re-created settlement is
// always approved from a request that was written again: its last-write time is
// a stable per-life stamp that the mail keys carry.
func (s *Service) submissionStamp(ctx context.Context, settlementID string) (time.Time, error) {
	sreq, err := s.dbRepo.GetSettlementRequest(ctx, settlementID)
	if err != nil {
		return time.Time{}, err
	}
	return sreq.UpdatedAt, nil
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

	requestedAt, err := s.submissionStamp(ctx, req.GetSettlementId())
	if err != nil {
		l.Error("failed to read the settlement request", zap.Error(err))
		return nil, err
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

		// The request document is what a re-created settlement is built from, so
		// it carries a copy of the counter: without it an owner who re-creates a
		// deleted settlement would start counting again at zero.
		if err := s.dbRepo.SetRequestNotifierReissues(ctx, req.GetSettlementId(), number); err != nil {
			return err
		}

		title, body := reissueMailText(*updated)
		return s.sendNotifierMail(ctx, *updated, recipient,
			model.NotifierReissueKey(updated.Id, requestedAt, number), title, body)
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

// ListNotifierPositions implements settlementv1.SettlementServiceServer. Public:
// the map draws one pin per standing block in a single call, and a placement
// discloses nothing Get does not already serve for one settlement.
//
// One read answers the whole list. Filling Settlement.notifier_position in List
// instead would cost one read per row, which is why that field stays unset there.
func (s *Service) ListNotifierPositions(
	ctx context.Context,
	_ *settlementv1.ListNotifierPositionsRequest,
) (*settlementv1.ListNotifierPositionsResponse, error) {
	placements, err := s.dbRepo.ListNotifiers(ctx)
	if err != nil {
		s.log.WithMethod("ListNotifierPositions").
			Error("failed to list notifier placements", zap.Error(err))
		return nil, err
	}

	return &settlementv1.ListNotifierPositionsResponse{
		Placements: s.mapper.ToNotifierPlacementsProto(placements),
	}, nil
}
