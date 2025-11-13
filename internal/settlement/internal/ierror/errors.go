package ierror

import "errors"

var (
	ErrAlreadyApproved         = errors.New("settlement request already approved")
	ErrInvalidArgument         = errors.New("invalid argument")
	ErrSettlementAlreadyHasTag = errors.New("settlement already has this tag")
	ErrTagNotActive            = errors.New("tag is not active")
	ErrTooManyTagsLimit        = errors.New("too many tags limit reached")
	ErrCreate                  = errors.New("failed to create settlement")
	ErrNotFound                = errors.New("settlement not found")
	ErrInvitationNotFound      = errors.New("invitation not found")
	ErrNotApproved             = errors.New("settlement request not approved")
	ErrUpdate                  = errors.New("failed to update settlement")
	ErrMaxTierReached          = errors.New("max tier reached")
	ErrInvalidSettlementType   = errors.New("invalid settlement type")
	ErrAlreadySubmitted        = errors.New("settlement request already submitted")
	ErrAlreadyMember           = errors.New("user is already a member of the settlement")
	ErrNotLeader               = errors.New("user is not a leader of this settlement")
	ErrPermissionDenied        = errors.New("permission denied")
)
