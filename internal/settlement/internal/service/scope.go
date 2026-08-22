package service

import (
	"context"
	"slices"
	"strings"

	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// manageScope gates settlement moderation.
const manageScope = "settlements:manage"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/settlement.v1.SettlementService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "Approve"):                 interceptor.Scope(manageScope),
		interceptor.Method(srvName + "ListPending"):             interceptor.Scope(manageScope),
		interceptor.Method(srvName + "Reject"):                  interceptor.Scope(manageScope),
		interceptor.Method(srvName + "AddTagToSettlement"):      interceptor.Scope("tags:manage"),
		interceptor.Method(srvName + "RemoveTagFromSettlement"): interceptor.Scope("tags:manage"),
		interceptor.Method(srvName + "AdminUpdateSettlement"):   interceptor.Scope(manageScope),
		interceptor.Method(srvName + "AddImperialFavor"):        interceptor.Scope(manageScope),
		interceptor.Method(srvName + "DeductImperialFavor"):     interceptor.Scope(manageScope),
		interceptor.Method(srvName + "ListImperialFavorLogs"):   interceptor.Scope(manageScope),
		interceptor.Method(srvName + "TransferImperialFavor"):   interceptor.ScopeAuthenticated,

		// Leader-gated: each of these calls IsLeaderOfSettlement on the target
		// settlement before mutating it. RemoveMember additionally accepts a
		// moderator holding manageScope (see requireScope).
		interceptor.Method(srvName + "UpdateSettlement"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "InviteMember"):     interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "RevokeInvitation"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetInvitations"):   interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "RemoveMember"):     interceptor.ScopeAuthenticated,

		// Self-service: scoped to the JWT subject.
		interceptor.Method(srvName + "Submit"):             interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "AcceptInvitation"):   interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "RejectInvitation"):   interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetUserInvitations"): interceptor.ScopeAuthenticated,

		// These take a user_id from the request without comparing it to the JWT
		// subject, so they disclose another player's settlement request state.
		// Left authenticated-only to keep this change behaviour-preserving; the
		// missing ownership check is tracked separately.
		interceptor.Method(srvName + "GetByUserId"):        interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "VerificationStatus"): interceptor.ScopeAuthenticated,
	}
}

// requireScope checks a scope in-handler, for methods where the requirement is
// conditional and so cannot be expressed in the Scope table: RemoveMember
// accepts either the settlement's leader or a moderator.
func (s *Service) requireScope(ctx context.Context, scope string) error {
	claims, err := interceptor.GetClaims(ctx)
	if err != nil {
		return status.Error(codes.Unauthenticated, "missing claims")
	}
	if !slices.Contains(strings.Fields(claims.Scope), scope) {
		return status.Error(codes.PermissionDenied, "caller is neither the settlement leader nor a moderator")
	}
	return nil
}
