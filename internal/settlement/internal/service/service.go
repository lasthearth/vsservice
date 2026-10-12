package service

import (
	"context"
	"errors"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Submit implements settlementv1.SettlementServiceServer
func (s *Service) Submit(ctx context.Context, req *settlementv1.SubmitRequest) (*settlementv1.SubmitResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.GetAttachments()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "attachments cannot be empty")
	}

	stype, err := TypeFromReqProto(req.GetType())
	if err != nil {
		s.log.Warn("unknown settlement type in submission", zap.Error(err), zap.String("user_id", userID))
		return nil, ierror.ErrInvalidSettlementType
	}
	kind := stype.Kind()

	// Every check of the input runs before the first read, so a malformed submission costs no query.
	attachs := make([]model.Attachment, len(req.GetAttachments()))
	for i, attachment := range req.GetAttachments() {
		if err := s.mediaUrl.Validate(attachment.GetUrl()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid attachment url")
		}
		attachs[i] = model.Attachment{
			Url:  attachment.GetUrl(),
			Desc: attachment.GetDescription(),
		}
	}

	s.log.Info("submitting settlement request",
		zap.String("leader_id", userID),
		zap.String("settlement_name", req.GetName()),
		zap.String("kind", string(kind)),
		zap.Int("attachments", len(req.GetAttachments())))

	// Each kind keeps its own request, so this lookup never mistakes a pending guild for a
	// settlement one and never levels a settlement up on a guild submission.
	found, err := s.dbRepo.GetSettlementRequestByLeader(ctx, userID, kind)
	switch {
	case err == nil:
	case errors.Is(err, ierror.ErrNotFound):
		found = nil
	default:
		s.log.Error("error checking for existing settlement request", zap.Error(err))
		return nil, err
	}

	// One read answers both questions the rules ask: whether the slot of this kind is free, and
	// whether the player may own a guild at all. The owner of a guild belongs to no settlement, and
	// that binds whoever asks to found one — a resubmission too, since its author owns the guild it
	// re-verifies.
	memberships, err := s.dbRepo.Memberships(ctx, userID)
	if err != nil {
		s.log.Error("failed to read the player's memberships", zap.Error(err))
		return nil, err
	}
	if kind == model.KindGuild {
		if err := memberships.MayOwnGuild(); err != nil {
			s.log.Info("guild submission refused", zap.Error(err), zap.String("user_id", userID))
			return nil, mapModelErr(err)
		}
	}
	// A first submission also needs the slot of its kind free. A resubmission does not: its author
	// already holds that membership, and the group it belongs to may have been deleted while the
	// request stayed.
	if found == nil {
		if err := memberships.MayJoin(kind); err != nil {
			s.log.Info("submission refused by the membership rules",
				zap.Error(err), zap.String("user_id", userID), zap.String("kind", string(kind)))
			return nil, mapModelErr(err)
		}
	}

	opts := SettlementOpts{
		Name: req.GetName(),
		Type: *stype,
		Leader: model.Member{
			UserId: userID,
		},
		Description: req.GetDescription(),
		Diplomacy:   req.GetDiplomacy(),
		Coordinates: model.Vector2{
			X: int(req.GetCoordinates().GetX()),
			Y: int(req.GetCoordinates().GetY()),
		},
		Attachments: attachs,
	}

	if found == nil {
		if err := s.dbRepo.CreateRequest(ctx, opts); err != nil {
			s.log.Error("failed to create new settlement request", zap.Error(err))
			return nil, err
		}

		return &settlementv1.SubmitResponse{}, nil
	}

	// User already has a request of this kind, handle level up or update.
	//
	// Checked before the notifier read: a pending request is refused outright, and
	// a first-time rejected one has no settlement behind it, so neither can have a
	// block standing. Both used to pay for a query whose result was thrown away.
	if found.Status == model.SettlementStatusPending {
		s.log.Info("request already submitted", zap.String("user_id", userID))
		return nil, status.Error(codes.AlreadyExists, "settlement request already pending")
	}

	// A guild keeps its tier, so LvlUp leaves it where it is and a further submission only
	// re-verifies the profile.
	if found.Status == model.SettlementStatusApproved {
		found.LvlUp()
		s.log.Debug("settlement level up",
			zap.String("before", string(opts.Type)),
			zap.String("after", string(found.Type)),
		)
	}

	// The settlement id is the request id. While its notifier block stands the
	// block's position is authoritative, and whatever the client sent is dropped.
	// Only a request that reached approval can have a settlement document, and only
	// a settlement can have a block, so a first-time rejected request skips the
	// read: it is guaranteed to come back empty. A guild skips it as well: its
	// herald carries no settlement binding and writes no placement, so there is
	// nothing to lock the coordinates to.
	coordinatesLocked := false
	if kind != model.KindGuild &&
		(found.Status == model.SettlementStatusApproved || found.Status == model.SettlementStatusUpdateRejected) {
		placement, hasNotifier, err := s.readNotifier(ctx, found.Id)
		if err != nil {
			return nil, err
		}
		if hasNotifier {
			opts.Coordinates, coordinatesLocked = model.ResolveCoordinates(opts.Coordinates, &placement)
		}
		if coordinatesLocked {
			// Deterministic, so a support request can be answered from the log alone:
			// the player typed coordinates and the block overrode them.
			s.log.Info("notifier position overrides the submitted coordinates",
				zap.String("settlement_id", found.Id),
				zap.Int("resolved_x", opts.Coordinates.X),
				zap.Int("resolved_y", opts.Coordinates.Y))
		}
	}

	opts.Type = found.Type
	if err := s.dbRepo.UpdateRequest(ctx, opts); err != nil {
		s.log.Error("failed to update settlement request", zap.Error(err))
		return nil, err
	}

	s.log.Info("settlement req created", zap.Int("attachment", len(attachs)))

	return &settlementv1.SubmitResponse{CoordinatesLocked: coordinatesLocked}, nil
}

// Get implements settlementv1.SettlementServiceServer
func (s *Service) Get(ctx context.Context, req *settlementv1.GetRequest) (*settlementv1.GetResponse, error) {
	s.log.Info("getting settlement details", zap.String("settlement_id", req.GetId()))

	settlement, err := s.dbRepo.GetSettlement(ctx, req.GetId())
	if err != nil {
		s.log.Error("failed to get settlement", zap.Error(err))
		return nil, err
	}

	if settlement == nil {
		return nil, status.Error(codes.NotFound, ierror.ErrNotFound.Error())
	}

	proto, err := s.settlementProto(ctx, *settlement)
	if err != nil {
		return nil, err
	}
	return &settlementv1.GetResponse{Settlement: proto}, nil
}

// List implements settlementv1.SettlementServiceServer
func (s *Service) List(ctx context.Context, req *settlementv1.ListRequest) (*settlementv1.ListResponse, error) {
	// An unspecified kind lists settlements, which is what the public map has always shown. A guild
	// is not a place on that map, so it takes an explicit GROUP_KIND_GUILD to list one.
	kind := KindFromProto(req.GetKind(), model.KindSettlement)
	s.log.Info("listing settlements", zap.String("kind", string(kind)))

	settlements, err := s.dbRepo.GetAllByKind(ctx, kind)
	if err != nil {
		s.log.Error("failed to list settlements", zap.Error(err))
		return nil, err
	}

	// notifier_position is deliberately unset here. It lives in its own
	// collection, so filling it would cost one extra read per row on the map's
	// full listing — an N+1 over every settlement the server has. The map reads a
	// single settlement with Get when it needs the block.
	return &settlementv1.ListResponse{
		Settlements: s.mapper.ToSettlementProtos(settlements),
	}, nil
}

// ListPending implements settlementv1.SettlementServiceServer
func (s *Service) ListPending(ctx context.Context, req *settlementv1.ListPendingRequest) (*settlementv1.ListPendingResponse, error) {
	// An unspecified kind lists both: the moderation queue must never hide a guild request.
	kind := KindFromProto(req.GetKind(), model.KindAny)
	s.log.Info("listing pending settlement requests", zap.String("kind", string(kind)))

	settlements, err := s.dbRepo.GetPendingSettlements(ctx, kind)
	if err != nil {
		s.log.Error("failed to list pending settlements", zap.Error(err))
		return nil, err
	}

	// These are requests, not settlements: no settlement document exists for a
	// pending one, so there is no notifier block and no position to fill.
	return &settlementv1.ListPendingResponse{
		Settlements: s.mapper.VerifsToSettlementProtos(settlements),
	}, nil
}

// Approve implements settlementv1.SettlementServiceServer
func (s *Service) Approve(ctx context.Context, req *settlementv1.ApproveRequest) (*settlementv1.ApproveResponse, error) {
	s.log.Info("approving settlement request", zap.String("settlement_id", req.GetId()))

	request, err := s.dbRepo.GetSettlementRequest(ctx, req.GetId())
	if err != nil {
		s.log.Error("failed to get settlement", zap.Error(err))
		return nil, err
	}

	if request == nil {
		return nil, ierror.ErrNotFound
	}

	if request.Status == model.SettlementStatusApproved {
		return nil, ierror.ErrAlreadyApproved
	}

	var result *ApprovalResult
	err = s.dbRepo.InTransaction(ctx, func(ctx context.Context) error {
		// Read inside the transaction, so an approval that raced ahead and already created the group
		// is visible here: this one then answers "already approved" from the repository's status
		// guard, instead of reporting a membership conflict the author did not cause.
		creating, err := s.isFirstApproval(ctx, req.GetId())
		if err != nil {
			return err
		}
		// Read here too, in the same unit of work as the creation and the mail, so a refusal rolls
		// the whole approval back. The author may have joined a group in the hours between the
		// submission and now.
		if err := s.ensureAuthorMayBeApproved(ctx, *request, creating); err != nil {
			s.log.Info("approval refused by the membership rules",
				zap.Error(err), zap.String("leader_id", request.Leader.UserId))
			return err
		}

		res, err := s.dbRepo.Approve(ctx, req.GetId())
		if err != nil {
			return err
		}
		result = res

		// The mail is written through the transaction's ctx, so it commits with
		// the request and the settlement or not at all.
		if res.Created {
			return s.deliverNotifier(ctx, *res)
		}
		return s.mailOwnersAboutUpgrade(ctx, *res)
	})
	if err != nil {
		s.log.Error("failed to approve settlement", zap.Error(err))
		return nil, err
	}

	// Site notifications are best effort and go out after the commit: a failing
	// notification must not undo an approval, and a rolled-back one must not
	// have notified anyone.
	s.notifyMembersAboutUpgrade(ctx, *result)

	return &settlementv1.ApproveResponse{}, nil
}

// isFirstApproval reports whether approving this request creates the group, which is the only case
// where the approval changes a membership and so has rules to answer. An upgrade changes none. The
// group id is the request id, so the absence of the group document is the answer.
func (s *Service) isFirstApproval(ctx context.Context, id string) (bool, error) {
	if _, err := s.dbRepo.GetSettlement(ctx, id); err != nil {
		if errors.Is(err, ierror.ErrNotFound) {
			return true, nil
		}
		s.log.Error("failed to look for the settlement behind the request", zap.Error(err))
		return false, err
	}
	return false, nil
}

// ensureAuthorMayBeApproved applies the membership rules to the author of a request that is about
// to create a group. Both rules of the kind are read at once: the slot must still be free, and a
// guild owner belongs to no settlement. The read happens here, in the caller's transaction, because
// the author may have joined a group in the hours between the submission and the approval.
func (s *Service) ensureAuthorMayBeApproved(ctx context.Context, request model.SettlementVerification, creating bool) error {
	if !creating {
		return nil
	}

	kind := request.Kind()
	m, err := s.dbRepo.Memberships(ctx, request.Leader.UserId)
	if err != nil {
		return err
	}
	if err := m.MayJoin(kind); err != nil {
		return mapModelErr(err)
	}
	if kind == model.KindGuild {
		return mapModelErr(m.MayOwnGuild())
	}
	return nil
}

// Reject implements settlementv1.SettlementServiceServer
func (s *Service) Reject(ctx context.Context, req *settlementv1.RejectRequest) (*settlementv1.RejectResponse, error) {
	s.log.Info("rejecting settlement request",
		zap.String("settlement_id", req.GetId()),
		zap.String("rejection_reason", req.GetRejectionReason()))

	if err := s.dbRepo.Reject(ctx, req.GetId(), req.GetRejectionReason()); err != nil {
		s.log.Error("failed to reject settlement", zap.Error(err))
		return nil, err
	}

	return &settlementv1.RejectResponse{}, nil
}

// RemoveMember implements settlementv1.SettlementServiceServer
func (s *Service) RemoveMember(ctx context.Context, req *settlementv1.RemoveMemberRequest) (*settlementv1.RemoveMemberResponse, error) {
	s.log.Info("removing member from settlement",
		zap.String("settlement_id", req.GetSettlementId()),
		zap.String("user_id", req.GetUserId()))

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, err
	}

	// Leader of THIS settlement, or a moderator holding the manage scope.
	// Without either check any authenticated caller could evict any member of
	// any settlement — the ids are readable from the public Get/List RPCs.
	if lerr := s.dbRepo.IsLeaderOfSettlement(ctx, req.GetSettlementId(), uid); lerr != nil {
		if serr := s.requireScope(ctx, manageScope); serr != nil {
			s.log.Warn("member removal denied: not leader and no manage scope",
				zap.String("user_id", uid), zap.Error(lerr))
			return nil, serr
		}
	}

	if err := s.dbRepo.RemoveMember(ctx, req.GetSettlementId(), req.GetUserId()); err != nil {
		s.log.Error("failed to remove member", zap.Error(err))
		return nil, err
	}

	return &settlementv1.RemoveMemberResponse{}, nil
}

// InviteMember implements settlementv1.SettlementServiceServer
func (s *Service) InviteMember(ctx context.Context, req *settlementv1.InviteMemberRequest) (*settlementv1.InviteMemberResponse, error) {
	s.log.Info("inviting member to settlement",
		zap.String("settlement_id", req.GetSettlementId()),
		zap.String("user_id", req.GetUserId()))

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	set, err := s.requirePermissionOn(ctx, req.GetSettlementId(), uid, model.PermInviteMember)
	if err != nil {
		return nil, err
	}

	// Answered before the invitation is written. An invitation the invited player could never
	// accept is worse than a refusal now, which is what storing it anyway used to produce. Holding
	// a membership of the other kind is fine, so a settlement member may be invited to a guild and
	// a guild member to a settlement; only a second membership of the same kind, and a guild owner
	// invited into a settlement, are refused.
	if err := s.dbRepo.EnsureMayJoin(ctx, req.GetUserId(), set.Kind()); err != nil {
		s.log.Info("invitation refused by the membership rules",
			zap.Error(err),
			zap.String("user_id", req.GetUserId()),
			zap.String("kind", string(set.Kind())))
		return nil, err
	}

	if err := s.dbRepo.CreateInvitation(ctx, req.GetSettlementId(), req.GetUserId()); err != nil {
		s.log.Error("failed to add member", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &settlementv1.InviteMemberResponse{}, nil
}

// GetInvitations implements settlementv1.SettlementServiceServer.
func (s *Service) GetInvitations(ctx context.Context, req *settlementv1.GetInvitationsRequest) (*settlementv1.GetInvitationsResponse, error) {
	l := s.log.
		With(zap.String("settlement_id", req.GetSettlementId())).
		WithMethod("get_invitations")

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		l.Error("failed to get user id", zap.Error(err))
		return nil, err
	}
	l = l.With(zap.String("user_id", uid))

	if err := s.requirePermission(ctx, req.GetSettlementId(), uid, model.PermInviteMember); err != nil {
		return nil, err
	}

	invitations, err := s.dbRepo.GetInvitations(ctx, req.GetSettlementId())
	if err != nil {
		l.Error("failed to get invitations", zap.Error(err))
		return nil, err
	}

	return &settlementv1.GetInvitationsResponse{
		Invitations: s.mapper.ToInvProtos(invitations),
	}, nil
}

// RevokeInvitation implements settlementv1.SettlementServiceServer.
func (s *Service) RevokeInvitation(ctx context.Context, req *settlementv1.RevokeInvitationRequest) (*settlementv1.RevokeInvitationResponse, error) {
	l := s.log.With(zap.String("method", "RevokeInvitation"), zap.String("settlement_id", req.GetSettlementId()), zap.String("invitation_id", req.GetInvitationId()))
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		l.Error("failed to get user id", zap.Error(err))
		return nil, err
	}
	l = l.With(zap.String("user_id", uid))

	if err := s.requirePermission(ctx, req.GetSettlementId(), uid, model.PermInviteMember); err != nil {
		return nil, err
	}

	if err := s.dbRepo.DeleteInvitationForLeader(ctx, req.GetInvitationId(), req.GetSettlementId()); err != nil {
		l.Error("failed to revoke invitation", zap.Error(err))
		return nil, err
	}

	return &settlementv1.RevokeInvitationResponse{}, nil
}

// AcceptInvitation implements settlementv1.SettlementServiceServer.
func (s *Service) AcceptInvitation(ctx context.Context, req *settlementv1.AcceptInvitationRequest) (*settlementv1.AcceptInvitationResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	// TODO: утекла бизнесуха в репу, придумать как вынести транкзакции и отрефакторить репу
	if err := s.dbRepo.AcceptInvitation(ctx, req.GetInvitationId(), uid); err != nil {
		s.log.Error("failed to accept invitation", zap.Error(err))
		return nil, err
	}

	return &settlementv1.AcceptInvitationResponse{}, nil
}

// RejectInvitation implements settlementv1.SettlementServiceServer.
func (s *Service) RejectInvitation(ctx context.Context, req *settlementv1.RejectInvitationRequest) (*settlementv1.RejectInvitationResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	invs, err := s.dbRepo.GetUserInvitations(ctx, uid)
	if err != nil {
		s.log.Error("failed to reject invitation", zap.Error(err))
		return nil, err
	}

	ids := lo.Map(invs, func(item model.Invitation, _ int) string {
		return item.Id
	})
	if !lo.Contains(ids, req.GetInvitationId()) {
		return nil, status.Errorf(codes.NotFound, "invitation for user %s not found", uid)
	}

	if err := s.dbRepo.DeleteInvitationForUser(ctx, req.GetInvitationId(), uid); err != nil {
		s.log.Error("failed to delete invitation", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &settlementv1.RejectInvitationResponse{}, nil
}

// GetUserInvitations implements settlementv1.SettlementServiceServer.
func (s *Service) GetUserInvitations(ctx context.Context, req *settlementv1.GetUserInvitationsRequest) (*settlementv1.GetUserInvitationsResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, err
	}

	if uid != req.GetUserId() {
		return nil, status.Errorf(codes.PermissionDenied, "user id mismatch")
	}

	invitations, err := s.dbRepo.GetUserInvitations(ctx, req.GetUserId())
	if err != nil {
		s.log.Error("failed to get user invitations", zap.Error(err))
		return nil, err
	}

	return &settlementv1.GetUserInvitationsResponse{
		Invitations: s.mapper.ToInvProtos(invitations),
	}, nil
}

// GetByUserId implements settlementv1.SettlementServiceServer. It answers the settlement only; a
// guild is served by GetUserMemberships.
func (s *Service) GetByUserId(ctx context.Context, req *settlementv1.GetByUserIdRequest) (*settlementv1.GetByUserIdResponse, error) {
	settlement, err := s.dbRepo.GetSettlementByUserId(ctx, req.GetUserId())
	if err != nil {
		if !errors.Is(err, ierror.ErrNotFound) {
			s.log.Error("failed to get settlements", zap.Error(err))
		}
		return nil, err
	}

	proto, err := s.settlementProto(ctx, *settlement)
	if err != nil {
		return nil, err
	}
	return &settlementv1.GetByUserIdResponse{Settlement: proto}, nil
}

// GetUserMemberships implements settlementv1.SettlementServiceServer: both groups a player belongs
// to, of which there is at most one of each kind.
func (s *Service) GetUserMemberships(ctx context.Context, req *settlementv1.GetUserMembershipsRequest) (*settlementv1.GetUserMembershipsResponse, error) {
	l := s.log.WithMethod("GetUserMemberships").With(zap.String("user_id", req.GetUserId()))

	out := &settlementv1.GetUserMembershipsResponse{}

	// Belonging to nothing is a normal answer, so a missing group leaves its field unset instead of
	// failing the call: the site renders one screen from both.
	settlement, err := s.dbRepo.GetSettlementByUserId(ctx, req.GetUserId())
	switch {
	case err == nil:
		proto, perr := s.settlementProto(ctx, *settlement)
		if perr != nil {
			return nil, perr
		}
		out.Settlement = proto
	case errors.Is(err, ierror.ErrNotFound):
	default:
		l.Error("failed to get the settlement", zap.Error(err))
		return nil, err
	}

	guild, err := s.dbRepo.GetGuildByUserId(ctx, req.GetUserId())
	switch {
	case err == nil:
		// settlementProto is what Get answers with too, and it is the one place that decides a guild
		// carries no notifier_position, so the two rpcs cannot disagree about it.
		proto, perr := s.settlementProto(ctx, *guild)
		if perr != nil {
			return nil, perr
		}
		out.Guild = proto
	case errors.Is(err, ierror.ErrNotFound):
	default:
		l.Error("failed to get the guild", zap.Error(err))
		return nil, err
	}

	return out, nil
}

// VerificationStatus implements settlementv1.SettlementServiceServer.
func (s *Service) VerificationStatus(ctx context.Context, req *settlementv1.VerificationStatusRequest) (*settlementv1.VerificationStatusResponse, error) {
	// Each kind keeps its own request; an unspecified kind answers the settlement one, which is
	// what this rpc answered before a guild could be submitted.
	kind := KindFromProto(req.GetKind(), model.KindSettlement)
	sreq, err := s.dbRepo.GetSettlementRequestByLeader(ctx, req.GetUserId(), kind)
	if err != nil {
		if !errors.Is(err, ierror.ErrNotFound) {
			s.log.Error("failed to get request status", zap.Error(err))
		}
		return nil, err
	}

	return &settlementv1.VerificationStatusResponse{
		Status:          string(sreq.Status),
		RejectionReason: sreq.RejectionReason,
	}, nil
}
