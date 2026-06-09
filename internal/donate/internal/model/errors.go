package model

import "errors"

var (
	errAlreadyRefunded     = errors.New("purchase already refunded")
	errCannotIssueRefunded = errors.New("cannot mark refunded purchase as issued")
)
