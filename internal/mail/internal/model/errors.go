package model

import "errors"

var (
	// ErrNothingToClaim is returned when Claim is called on a notification (a
	// mail with no attachments).
	ErrNothingToClaim = errors.New("nothing to claim")
	// ErrMailClaimTerminal is returned when a transition is attempted from a
	// terminal state.
	ErrMailClaimTerminal = errors.New("mail claim is in a terminal state")
)
