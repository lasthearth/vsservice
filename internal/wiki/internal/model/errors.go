package model

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

// Domain errors of the wiki. They carry gRPC status codes, so the service can
// return them as they are.
var (
	// errTitleRequired reports an article without a heading.
	errTitleRequired = ierror.InvalidArgument("title is required")
	// errSlugInvalid reports a slug that is not a usable URL segment.
	errSlugInvalid = ierror.InvalidArgument("slug must be lowercase latin words joined by dashes")
	// errSectionUnknown reports a section the wiki does not have.
	errSectionUnknown = ierror.InvalidArgument("unknown section")
	// errStatusUnknown reports a publication state the wiki does not have.
	errStatusUnknown = ierror.InvalidArgument("unknown status")

	// ErrVersionConflict reports an edit based on an outdated article.
	ErrVersionConflict = ierror.FailedPrecondition("article was changed by someone else, reload it")
	// ErrSlugTaken reports a slug already used by another article.
	ErrSlugTaken = ierror.AlreadyExists("slug is already taken")
	// ErrNotFound reports an article that does not exist.
	ErrNotFound = ierror.NotFound("article not found")
	// ErrRevisionNotFound reports a revision that does not exist.
	ErrRevisionNotFound = ierror.NotFound("revision not found")
	// ErrSuggestionNotFound reports a proposal that does not exist.
	ErrSuggestionNotFound = ierror.NotFound("suggestion not found")
	// ErrSuggestionReviewed reports a proposal that was already decided.
	ErrSuggestionReviewed = ierror.FailedPrecondition("suggestion was already reviewed")
	// ErrTooManySuggestions reports an author with too many proposals waiting.
	ErrTooManySuggestions = ierror.ResourceExhausted("too many suggestions waiting for review")
	// ErrReasonRequired reports a rejection without a reason.
	ErrReasonRequired = ierror.InvalidArgument("rejection reason is required")
)
