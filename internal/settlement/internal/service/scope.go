package service

import (
	"context"
	"slices"
	"strings"

	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
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

		// Join requests, roles, ownership transfer, contact info, leaving —
		// gated in-handler via requirePermission / requireOwner on the target
		// settlement.
		interceptor.Method(srvName + "CreateJoinRequest"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "CancelJoinRequest"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetMyJoinRequests"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ListJoinRequests"):   interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ApproveJoinRequest"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "RejectJoinRequest"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "CreateRole"):         interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "UpdateRole"):         interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "DeleteRole"):         interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "AssignRole"):         interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "UnassignRole"):       interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "TransferOwnership"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "LeaveSettlement"):    interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "UpdateContactInfo"):  interceptor.ScopeAuthenticated,

		// Admin-only owner and moderation operations.
		interceptor.Method(srvName + "AddOwner"):         interceptor.Scope(manageScope),
		interceptor.Method(srvName + "RemoveOwner"):      interceptor.Scope(manageScope),
		interceptor.Method(srvName + "SetRolesEnabled"):  interceptor.Scope(manageScope),
		interceptor.Method(srvName + "DeleteSettlement"): interceptor.Scope(manageScope),

		// Self-service: scoped to the JWT subject.
		interceptor.Method(srvName + "Submit"):             interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "AcceptInvitation"):   interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "RejectInvitation"):   interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetUserInvitations"): interceptor.ScopeAuthenticated,

		// Look up by a user_id from the request, no subject comparison.
		//
		// GetByUserId returns a Settlement, which Get and List already serve
		// publicly (see matcher.go), so it discloses nothing those do not.
		//
		// VerificationStatus returns the status and rejection_reason of that
		// user's settlement request, which has no public equivalent —
		// ListPending is manage-gated. Any authenticated caller can therefore
		// read whether a given player applied and why they were rejected. Left
		// authenticated-only to keep this change behaviour-preserving; the
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

// requirePermission loads the settlement and checks that userID may perform an
// action requiring perm (owner passes unconditionally).
func (s *Service) requirePermission(ctx context.Context, settlementID, userID string, perm model.Permission) error {
	set, err := s.dbRepo.GetSettlement(ctx, settlementID)
	if err != nil {
		return err
	}
	if !set.HasPermission(userID, perm) {
		return ierror.ErrNotLeader
	}
	return nil
}

// requireOwner loads the settlement and checks that userID holds the owner role.
func (s *Service) requireOwner(ctx context.Context, settlementID, userID string) (*model.Settlement, error) {
	set, err := s.dbRepo.GetSettlement(ctx, settlementID)
	if err != nil {
		return nil, err
	}
	if !set.IsOwner(userID) {
		return nil, ierror.ErrNotLeader
	}
	return set, nil
}
