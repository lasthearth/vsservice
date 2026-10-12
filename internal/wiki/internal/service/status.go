package service

import (
	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
)

// StatusToProto converts a domain publication state to its protobuf value.
//
// An empty or unknown state maps to UNSPECIFIED, which the service reads as
// "every status" on a list request.
func StatusToProto(status string) wikiv1.ArticleStatus {
	switch status {
	case model.StatusDraft:
		return wikiv1.ArticleStatus_ARTICLE_STATUS_DRAFT
	case model.StatusPublished:
		return wikiv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED
	default:
		return wikiv1.ArticleStatus_ARTICLE_STATUS_UNSPECIFIED
	}
}

// ProtoToStatus converts a protobuf publication state to its domain value.
//
// UNSPECIFIED maps to an empty string, which the service reads as "every
// status" rather than as a state of its own.
func ProtoToStatus(status wikiv1.ArticleStatus) string {
	switch status {
	case wikiv1.ArticleStatus_ARTICLE_STATUS_DRAFT:
		return model.StatusDraft
	case wikiv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED:
		return model.StatusPublished
	default:
		return ""
	}
}

// SuggestionStatusToProto converts a domain review state to its protobuf value.
func SuggestionStatusToProto(status string) wikiv1.SuggestionStatus {
	switch status {
	case model.SuggestionPending:
		return wikiv1.SuggestionStatus_SUGGESTION_STATUS_PENDING
	case model.SuggestionApproved:
		return wikiv1.SuggestionStatus_SUGGESTION_STATUS_APPROVED
	case model.SuggestionRejected:
		return wikiv1.SuggestionStatus_SUGGESTION_STATUS_REJECTED
	default:
		return wikiv1.SuggestionStatus_SUGGESTION_STATUS_UNSPECIFIED
	}
}

// SuggestionKindToProto converts a domain proposal kind to its protobuf value.
func SuggestionKindToProto(kind string) wikiv1.SuggestionKind {
	switch kind {
	case model.SuggestionNewArticle:
		return wikiv1.SuggestionKind_SUGGESTION_KIND_NEW_ARTICLE
	case model.SuggestionArticleEdit:
		return wikiv1.SuggestionKind_SUGGESTION_KIND_ARTICLE_EDIT
	default:
		return wikiv1.SuggestionKind_SUGGESTION_KIND_UNSPECIFIED
	}
}
