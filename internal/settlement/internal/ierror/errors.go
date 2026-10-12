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
	ErrAlreadyInGuild          = ierror.AlreadyExists("user is already a member of a guild")
	// ErrJoinRequestExists answers a second application to the same group. It is the unique
	// (user_id, settlement_id) index on the applications that reports it, not a membership.
	ErrJoinRequestExists = ierror.AlreadyExists("an application to this group already exists")
	// ErrGuildOwnerInSettlement is the one rule that crosses the two kinds: the owner of a guild
	// belongs to no settlement. It answers both directions — a guild owner who asks to join a
	// settlement, and a settlement member who asks to found or own a guild.
	ErrGuildOwnerInSettlement = ierror.FailedPrecondition("the owner of a guild cannot belong to a settlement")
	ErrNotLeader              = ierror.PermissionDenied("user is not a leader of this settlement")
	ErrPermissionDenied       = ierror.PermissionDenied("permission denied")
	ErrJoinRequestLimit       = ierror.ResourceExhausted("active join-request limit reached")
	ErrLastOwner              = ierror.FailedPrecondition("cannot remove the last owner")
	// ErrDisbandRequired answers the last owner of a guild who is also its only member and sent no
	// confirmation. Leaving that guild ends it, so the site asks once more and repeats the call with
	// disband set.
	ErrDisbandRequired = ierror.FailedPrecondition("the last owner of a guild leaves only by disbanding it")
	// ErrDisbandConflict answers when the guild gained a member between the read and the delete. The
	// caller must transfer the ownership to that member instead.
	ErrDisbandConflict     = ierror.FailedPrecondition("the guild has another member now: transfer the ownership instead")
	ErrRolesDisabled       = ierror.FailedPrecondition("roles are disabled for this settlement")
	ErrRoleLimit           = ierror.ResourceExhausted("role limit reached")
	ErrRoleNotFound        = ierror.NotFound("role not found")
	ErrMemberNotFound      = ierror.NotFound("member not found")
	ErrAlreadyOwner        = ierror.FailedPrecondition("member is already an owner")
	ErrInviteLinkNotFound  = ierror.NotFound("invite link not found")
	ErrInviteLinkLimit     = ierror.ResourceExhausted("active invite link limit reached")
	ErrInviteLinkExpired   = ierror.FailedPrecondition("invite link has expired")
	ErrInviteLinkExhausted = ierror.FailedPrecondition("invite link has no uses left")
	ErrNotifierUnavailable = ierror.FailedPrecondition("settlement type has no notifier block")
	ErrNoOwnerToDeliver    = ierror.FailedPrecondition("recipient is not an owner of the settlement")
	ErrInviteLinkRevoked   = ierror.FailedPrecondition("invite link was revoked")

	// ErrNotifierRead reports that the position of a settlement's notifier block
	// could not be read. "No block placed" is not this error: that is a normal
	// answer and leaves notifier_position unset. A decode or connection failure is,
	// and it fails closed — serving an unset position would render the upgrade form
	// editable and let Submit drop the coordinates the player typed.
	ErrNotifierRead = ierror.Unavailable("failed to read the settlement notifier position")
)
