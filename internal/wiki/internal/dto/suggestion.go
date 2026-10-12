package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// SuggestionCollection is the MongoDB collection holding player proposals.
const SuggestionCollection = "wiki_suggestions"

// Suggestion is the stored shape of a proposal.
//
// The proposed article is stored the same way an article is, so applying a
// proposal is a conversion rather than a reassembly.
type Suggestion struct {
	// Embedded, not named: mongox.UpdateDoc locates the version guard by
	// reflecting on an anonymous Model field, and it needs the promoted
	// envelope methods.
	mongox.Model `bson:",inline"`

	Kind             string     `bson:"kind"`
	ArticleId        string     `bson:"article_id,omitempty"`
	Proposal         Article    `bson:"proposal"`
	Status           string     `bson:"status"`
	Note             string     `bson:"note,omitempty"`
	AuthorName       string     `bson:"author_name,omitempty"`
	ReviewerName     string     `bson:"reviewer_name,omitempty"`
	RejectReason     string     `bson:"reject_reason,omitempty"`
	AppliedArticleId string     `bson:"applied_article_id,omitempty"`
	ReviewedAt       *time.Time `bson:"reviewed_at,omitempty"`
}

// Id returns the document identifier, which the pagination helper needs.
func (s Suggestion) Id() bson.ObjectID {
	return s.Model.Id
}
