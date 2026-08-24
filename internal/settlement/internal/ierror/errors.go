package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrAlreadyApproved         = ierror.FailedPrecondition("settlement request already approved")
	ErrInvalidArgument         = ierror.InvalidArgument("invalid argument")
	ErrSettlementAlreadyHasTag = ierror.AlreadyExists("settlement already has this tag")
	ErrTagNotActive            = ierror.FailedPrecondition("tag is not active")
	ErrTooManyTagsLimit        = ierror.ResourceExhausted("too many tags limit reached")
	ErrCreate                  = ierror.Internal("failed to create settlement")
	ErrNotFound                = ierror.NotFound("settlement not found")
	ErrInvitationNotFound      = ierror.NotFound("invitation not found")
	ErrNotApproved             = ierror.FailedPrecondition("settlement request not approved")
	ErrUpdate                  = ierror.Internal("failed to update settlement")
	ErrMaxTierReached          = ierror.FailedPrecondition("max tier reached")
	ErrInvalidSettlementType   = ierror.InvalidArgument("invalid settlement type")
	ErrAlreadySubmitted        = ierror.AlreadyExists("settlement request already submitted")
	ErrAlreadyMember           = ierror.AlreadyExists("user is already a member of the settlement")
	ErrNotLeader               = ierror.PermissionDenied("user is not a leader of this settlement")
	ErrPermissionDenied        = ierror.PermissionDenied("permission denied")
	ErrJoinRequestLimit        = ierror.ResourceExhausted("active join-request limit reached")
	ErrLastOwner               = ierror.FailedPrecondition("cannot remove the last owner")
	ErrRolesDisabled           = ierror.FailedPrecondition("roles are disabled for this settlement")
	ErrRoleLimit               = ierror.ResourceExhausted("role limit reached")
	ErrRoleNotFound            = ierror.NotFound("role not found")
	ErrMemberNotFound          = ierror.NotFound("member not found")
	ErrAlreadyOwner            = ierror.FailedPrecondition("member is already an owner")
)
