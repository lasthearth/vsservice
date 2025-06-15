package repoerr

import "github.com/go-faster/errors"

var (
	ErrCreate                = errors.New("failed to create settlement")
	ErrNotFound              = errors.New("settlement not found")
	ErrUpdate                = errors.New("failed to update settlement")
	ErrMaxTierReached        = errors.New("max tier reached")
	ErrInvalidSettlementType = errors.New("invalid settlement type")
	ErrAlreadySubmitted      = errors.New("settlement request already submitted")
	ErrAlreadyMember         = errors.New("user is already a member of the settlement")
	ErrNotLeader             = errors.New("user is not a leader of this settlement")
)
