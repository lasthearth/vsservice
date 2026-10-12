package service

import (
	"context"
	"slices"
	"strings"
	"time"

	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
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

	if articleId != "" {
		// An edit proposal has to name an article that exists, otherwise the
		// queue would hold a change nobody can apply.
		if _, err := s.repo.Get(ctx, articleId); err != nil {
			return nil, err
		}
	}

	pending, err := s.repo.CountPendingSuggestions(ctx, author)
	if err != nil {
		return nil, err
	}
	if pending >= model.MaxPendingSuggestions {
		return nil, model.ErrTooManySuggestions
	}

	suggestion, err := model.NewSuggestion(model.SuggestionInput{
		ArticleId: articleId,
		Proposal:  s.fromProto(input.GetProposal()),
		Note:      input.GetNote(),
	}, author, time.Now())
	if err != nil {
		return nil, err
	}

	created, err := s.repo.CreateSuggestion(ctx, suggestion)
	if err != nil {
		return nil, err
	}

	return s.mapper.ToSuggestionProto(*created), nil
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
		suggestions = append(suggestions, s.mapper.ToSuggestionProto(*suggestion))
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
	if !suggestion.IsPending() {
		return nil, model.ErrSuggestionReviewed
	}

	applied, err := s.applySuggestion(ctx, suggestion, reviewer)
	if err != nil {
		return nil, err
	}

	if err := suggestion.Approve(reviewer, applied.Envelope.Id, time.Now()); err != nil {
		return nil, err
	}

	saved, err := s.repo.SaveSuggestion(ctx, suggestion)
	if err != nil {
		return nil, err
	}

	return s.mapper.ToSuggestionProto(*saved), nil
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

	return s.mapper.ToSuggestionProto(*saved), nil
}

// applySuggestion turns an approved proposal into an article.
//
// A proposal for a new page is created as a published article, because approval
// is the editor's decision to publish. A proposal for an existing page is
// applied as an edit: the state being replaced is saved as a revision first, so
// an approved edit can be undone like any other.
func (s *Service) applySuggestion(
	ctx context.Context,
	suggestion *model.Suggestion,
	reviewer string,
) (*model.Article, error) {
	now := time.Now()
	proposal := suggestion.Proposal

	if suggestion.Kind == model.SuggestionArticleEdit {
		article, err := s.repo.Get(ctx, suggestion.ArticleId)
		if err != nil {
			return nil, err
		}

		proposal.AdoptDocument(article.Envelope)
		proposal.Publish()
		article.SaveRevision("Утверждённое предложение игрока", now)

		if err := article.ApplyEdit(proposal, reviewer, 0, now); err != nil {
			return nil, err
		}

		return s.repo.Save(ctx, article, 0)
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

	proposal.Publish()

	created, err := s.repo.Create(ctx, &proposal)
	if err != nil {
		return nil, err
	}

	return created, nil
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
