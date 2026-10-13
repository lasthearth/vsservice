package service

import (
	"context"
	"slices"
	"strings"
	"time"

	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"go.uber.org/zap"
)

// SubmitSuggestion implements wikiv1.WikiServiceServer.
//
// Nothing is published here: the proposal is validated by the article rules and
// then parked for review. Validation at the door matters, because a proposal
// that could never be published would otherwise waste an editor's time.
func (s *Service) SubmitSuggestion(
	ctx context.Context,
	req *wikiv1.SubmitSuggestionRequest,
) (*wikiv1.Suggestion, error) {
	author, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	input := req.GetSuggestion()
	articleId := input.GetArticleId()

	var base *model.Article

	if articleId != "" {
		// An edit proposal has to name an article that exists, otherwise the
		// queue would hold a change nobody can apply. The article is kept: the
		// proposal remembers the version and the text it was written against, so
		// a later approval can tell that the page moved on.
		stored, err := s.repo.Get(ctx, articleId)
		if err != nil {
			return nil, err
		}

		base = stored
	}

	pending, err := s.repo.CountPendingSuggestions(ctx, author)
	if err != nil {
		return nil, err
	}
	if pending >= model.MaxPendingSuggestions {
		return nil, model.ErrTooManySuggestions
	}

	suggestion, err := model.NewSuggestion(model.SuggestionInput{
		ArticleId:   articleId,
		Proposal:    s.fromProto(input.GetProposal()),
		Note:        input.GetNote(),
		BaseVersion: input.GetBaseVersion(),
		Base:        base,
	}, author, time.Now())
	if err != nil {
		return nil, err
	}

	created, err := s.repo.CreateSuggestion(ctx, suggestion)
	if err != nil {
		return nil, err
	}

	return s.toSuggestionProto(created), nil
}

// ListSuggestions implements wikiv1.WikiServiceServer.
//
// An author sees only their own proposals; a reviewer sees every one of them.
func (s *Service) ListSuggestions(
	ctx context.Context,
	req *wikiv1.ListSuggestionsRequest,
) (*wikiv1.ListSuggestionsResponse, error) {
	author, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	filter := model.SuggestionFilter{Status: ProtoToSuggestionStatus(req.GetStatus())}
	if !s.canReview(ctx) {
		filter.AuthorName = author
	}

	limit := int64(req.GetPageSize())
	if limit <= 0 {
		limit = defaultPageSize
	}
	limit = min(limit, maxPageSize)

	page, err := s.repo.ListSuggestions(ctx, filter, req.GetPageToken(), limit)
	if err != nil {
		return nil, err
	}

	suggestions := make([]*wikiv1.Suggestion, 0, len(page.Suggestions))
	for _, suggestion := range page.Suggestions {
		suggestions = append(suggestions, s.toSuggestionProto(suggestion))
	}

	return &wikiv1.ListSuggestionsResponse{
		Suggestions:   suggestions,
		NextPageToken: page.Next,
	}, nil
}

// ApproveSuggestion implements wikiv1.WikiServiceServer.
//
// The proposal is applied through the article domain, so an approved proposal
// goes through exactly the same rules — slug uniqueness, validation, history —
// as an edit an editor makes directly.
//
// The decision is reserved in storage before the article is written, because the
// two collections are not one transaction: reserving first is what makes the
// proposal apply exactly once, and a failed apply gives the reservation back.
func (s *Service) ApproveSuggestion(
	ctx context.Context,
	req *wikiv1.ApproveSuggestionRequest,
) (*wikiv1.Suggestion, error) {
	reviewer, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	suggestion, err := s.repo.GetSuggestion(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err := suggestion.Approve(reviewer, time.Now()); err != nil {
		return nil, err
	}

	// SaveSuggestion is pinned to the pending state, so of two editors approving
	// one proposal only one gets past this line: the loser is told it was
	// already reviewed and never reaches the wiki.
	reserved, err := s.repo.SaveSuggestion(ctx, suggestion)
	if err != nil {
		return nil, err
	}

	applied, err := s.applySuggestion(ctx, reserved, req.GetAllowStale())
	if err != nil {
		if reopenErr := s.repo.ReopenSuggestion(ctx, reserved.Id); reopenErr != nil {
			s.logger.Error("wiki suggestion left approved after a failed apply",
				zap.String("id", reserved.Id),
				zap.Error(reopenErr),
			)
		}

		return nil, err
	}

	if err := s.repo.SetAppliedSuggestionArticle(ctx, reserved.Id, applied.Envelope.Id); err != nil {
		return nil, err
	}

	// The author hears about the decision once it is stored, never before.
	title, message := approvedText(reserved, applied)
	s.notifyDecision(ctx, reserved, title, message)

	saved, err := s.repo.GetSuggestion(ctx, reserved.Id)
	if err != nil {
		return nil, err
	}

	return s.toSuggestionProto(saved), nil
}

// RejectSuggestion implements wikiv1.WikiServiceServer.
func (s *Service) RejectSuggestion(
	ctx context.Context,
	req *wikiv1.RejectSuggestionRequest,
) (*wikiv1.Suggestion, error) {
	reviewer, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	reason := req.GetReason()
	if reason == "" {
		return nil, model.ErrReasonRequired
	}

	suggestion, err := s.repo.GetSuggestion(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err := suggestion.Reject(reviewer, reason, time.Now()); err != nil {
		return nil, err
	}

	saved, err := s.repo.SaveSuggestion(ctx, suggestion)
	if err != nil {
		return nil, err
	}

	// The author hears about the decision once it is stored, never before.
	title, message := rejectedText(saved)
	s.notifyDecision(ctx, saved, title, message)

	return s.toSuggestionProto(saved), nil
}

// applySuggestion turns an approved proposal into an article.
//
// A proposal for a new page is created as a published article, because approval
// is the editor's decision to publish. A proposal for an existing page is
// applied as an edit: the state being replaced is saved as a revision first, so
// an approved edit can be undone like any other.
//
// An edit written against a version the article has since left is refused
// unless allowStale is set, which is the reviewer saying they saw what would be
// replaced.
func (s *Service) applySuggestion(
	ctx context.Context,
	suggestion *model.Suggestion,
	allowStale bool,
) (*model.Article, error) {
	now := time.Now()
	proposal := suggestion.Proposal

	if suggestion.Kind == model.SuggestionArticleEdit {
		article, err := s.repo.Get(ctx, suggestion.ArticleId)
		if err != nil {
			return nil, err
		}

		// The proposal was written against an older state of the page, so applying
		// it now would drop whatever the editor saved in between. The check sits
		// here rather than before the reservation, so nothing can change the page
		// between it and the write — which is itself pinned to the version read
		// above and reports a conflict rather than overwriting.
		if suggestion.IsStaleAgainst(article) && !allowStale {
			return nil, model.ErrSuggestionStale
		}

		// The write is pinned to the version it was read from, so an editor who
		// saves the same page while a proposal is approved is reported as a
		// conflict instead of being silently replaced.
		expected := article.Envelope.Version

		proposal.AdoptDocument(article.Envelope)
		proposal.Publish()
		article.SaveRevision("Утверждённое предложение игрока", now)

		if err := article.ApplyEdit(proposal, suggestion.AuthorName, expected, now); err != nil {
			return nil, err
		}

		return s.repo.Save(ctx, article, expected)
	}

	// A new page is published by the approval itself; the slug has to be free,
	// which the unique index also guarantees.
	taken, err := s.repo.ExistsSlug(ctx, proposal.Slug, "")
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, model.ErrSlugTaken
	}

	// The proposal's writer is the page's author; the reviewer only decided to
	// publish it.
	proposal.AssignAuthor(suggestion.AuthorName)
	proposal.Publish()

	created, err := s.repo.Create(ctx, &proposal)
	if err != nil {
		return nil, err
	}

	return created, nil
}

// toSuggestionProto converts a proposal for the wire and attaches the article
// its author wrote against. The converter leaves that field alone — see the
// Mapper interface — so it is filled in here.
func (s *Service) toSuggestionProto(suggestion *model.Suggestion) *wikiv1.Suggestion {
	proto := s.mapper.ToSuggestionProto(*suggestion)
	proto.Base = s.toProto(suggestion.Base)

	return proto
}

// canReview reports whether the caller is an editor.
//
// The review scope is checked by the interceptor before a review call arrives;
// this is for the reads that both roles may make, where an editor sees the
// whole queue and an author only their own proposals. Logto sends scopes as one
// space-separated claim, the same way the interceptor reads it.
func (s *Service) canReview(ctx context.Context) bool {
	claims, err := interceptor.GetClaims(ctx)
	if err != nil {
		return false
	}

	return slices.Contains(strings.Fields(claims.Scope), scopeWikiReview)
}

// scopeWikiReview is the scope an editor needs to decide a proposal. It is
// declared here because the review paths read it from the token as well as
// declare it in Scope().
const scopeWikiReview = "wiki:review"

// protoToSuggestionStatus converts a protobuf review state to its domain value.
func ProtoToSuggestionStatus(status wikiv1.SuggestionStatus) string {
	switch status {
	case wikiv1.SuggestionStatus_SUGGESTION_STATUS_PENDING:
		return model.SuggestionPending
	case wikiv1.SuggestionStatus_SUGGESTION_STATUS_APPROVED:
		return model.SuggestionApproved
	case wikiv1.SuggestionStatus_SUGGESTION_STATUS_REJECTED:
		return model.SuggestionRejected
	default:
		return ""
	}
}
