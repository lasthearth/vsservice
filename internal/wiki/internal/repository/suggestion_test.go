package repository

import (
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/wiki/internal/dto"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"github.com/lasthearth/vsservice/internal/wiki/internal/repository/repomapper"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestSuggestionBaseSurvivesTheDocumentRoundTrip pins what a reviewer reads
// back: the article a proposal was written against keeps its identity,
// version and timestamps through the stored document. The mapper ignores the
// envelope of nested articles, so the write stamps it onto the base
// subdocument and the read applies it again — without both halves the base
// returns with an empty id and version 0 next to a populated base_version.
func TestSuggestionBaseSurvivesTheDocumentRoundTrip(t *testing.T) {
	r := &Repository{mapper: &repomapper.MapperImpl{}}

	now := time.Now().UTC().Truncate(time.Millisecond)
	articleID := bson.NewObjectID().Hex()

	base, err := model.NewArticle(model.ArticleInput{
		Title:   "Поселения",
		Section: model.SectionSettlements,
		Body:    "## Роли",
	}, "author-1", now)
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}
	base.AdoptDocument(model.Envelope{
		Id:        articleID,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   7,
	})

	suggestion, err := model.NewSuggestion(model.SuggestionInput{
		ArticleId: articleID,
		Proposal: model.ArticleInput{
			Title:   "Поселения и роли",
			Section: model.SectionSettlements,
			Body:    "## Роли и налоги",
		},
		Base:        base,
		BaseVersion: 7,
	}, "author-2", now)
	if err != nil {
		t.Fatalf("NewSuggestion: %v", err)
	}

	stored := r.toStoredSuggestion(suggestion)

	// Through BSON, the way the document is written and read back.
	raw, err := bson.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded dto.Suggestion
	if err := bson.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := r.suggestionToModel(decoded)
	if got.Base == nil {
		t.Fatal("the stored proposal kept no base")
	}
	if got.Base.Envelope.Id != articleID {
		t.Errorf("base id = %q, want %q", got.Base.Envelope.Id, articleID)
	}
	if got.Base.Envelope.Version != 7 {
		t.Errorf("base version = %d, want 7", got.Base.Envelope.Version)
	}
	if !got.Base.Envelope.CreatedAt.Equal(now) {
		t.Errorf("base created_at = %v, want %v", got.Base.Envelope.CreatedAt, now)
	}
	if !got.Base.Envelope.UpdatedAt.Equal(now) {
		t.Errorf("base updated_at = %v, want %v", got.Base.Envelope.UpdatedAt, now)
	}
	if got.Base.Title != "Поселения" {
		t.Errorf("base title = %q, want the article as the author saw it", got.Base.Title)
	}
}
