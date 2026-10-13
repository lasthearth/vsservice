package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"go.uber.org/zap"
)

// Notifier tells a player about a decision on their proposal.
type Notifier interface {
	Notify(ctx context.Context, userId, title, message string) error
}

// createNotifier adapts the notification use case to Notifier.
type createNotifier struct {
	uc *notificationuc.Create
}

func (n createNotifier) Notify(ctx context.Context, userId, title, message string) error {
	return n.uc.CreateNotification(ctx, title, message, notificationuc.WithUserId(userId))
}

// NewNotifier builds the wiki notifier over the notification use case.
func NewNotifier(uc *notificationuc.Create) Notifier {
	return createNotifier{uc: uc}
}

// The player-facing texts of a decision. A notification carries no link field,
// so the address of the page is part of the message.
const (
	wikiSiteUrl = "https://lasthearth.ru/wiki/"

	titleArticlePublished  = "Статья опубликована"
	titleEditAccepted      = "Правка принята"
	titleSuggestionRefused = "Предложение отклонено"
)

// notifyDecision tells the author what happened to their proposal.
//
// A notification that cannot be sent is logged, never returned: the decision is
// already stored, and undoing it because a message failed would lose the
// editor's work over something a retry could not recover anyway.
func (s *Service) notifyDecision(ctx context.Context, suggestion *model.Suggestion, title, message string) {
	if err := s.notifier.Notify(ctx, suggestion.AuthorName, title, message); err != nil {
		s.logger.Warn("wiki decision notification not delivered",
			zap.String("suggestion_id", suggestion.Id),
			zap.Error(err),
		)
	}
}

// approvedText is what the author of an approved proposal reads. The address of
// an edit comes from the article it produced, because approving it is a save
// like any other and may have moved the page.
func approvedText(suggestion *model.Suggestion, applied *model.Article) (string, string) {
	if suggestion.Kind == model.SuggestionArticleEdit {
		return titleEditAccepted,
			"Ваша правка статьи «" + suggestion.Proposal.Title + "» применена: " + wikiSiteUrl + applied.Slug
	}

	return titleArticlePublished,
		"Ваше предложение «" + suggestion.Proposal.Title + "» одобрено: " + wikiSiteUrl + suggestion.Proposal.Slug
}

// rejectedText is what the author of a refused proposal reads: the page and the
// reason the reviewer gave them.
func rejectedText(suggestion *model.Suggestion) (string, string) {
	return titleSuggestionRefused, "«" + suggestion.Proposal.Title + "»: " + suggestion.RejectReason
}
