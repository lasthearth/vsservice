package model

import (
	"errors"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

func draft() Article {
	return Article{Title: "Как объявить набег", Section: SectionWar, Body: "## Шаги"}
}

func created(t *testing.T) *Article {
	t.Helper()

	article, err := NewArticle(draft(), "author-1", now)
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}
	article.Envelope.Id = "article-1"

	return article
}

func TestNewArticleFillsDefaults(t *testing.T) {
	article, err := NewArticle(Article{Title: "Поселения"}, "author-1", now)
	if err != nil {
		t.Fatalf("NewArticle: %v", err)
	}

	if article.Slug != "poseleniya" {
		t.Errorf("slug = %q, want %q", article.Slug, "poseleniya")
	}
	if article.Status != StatusDraft {
		t.Errorf("status = %q, want %q", article.Status, StatusDraft)
	}
	if article.Section != SectionStart {
		t.Errorf("section = %q, want %q", article.Section, SectionStart)
	}
	if article.Envelope.Version != 1 {
		t.Errorf("version = %d, want 1", article.Envelope.Version)
	}
	if !article.Envelope.CreatedAt.Equal(now) {
		t.Errorf("createdAt = %v, want %v", article.Envelope.CreatedAt, now)
	}
}

func TestNewArticleRejectsEmptyTitle(t *testing.T) {
	_, err := NewArticle(Article{Title: ""}, "author-1", now)
	if !errors.Is(err, errTitleRequired) {
		t.Fatalf("err = %v, want title required", err)
	}
}

func TestNewArticleRejectsUnknownSection(t *testing.T) {
	_, err := NewArticle(Article{Title: "Статья", Section: "nope"}, "author-1", now)
	if !errors.Is(err, errSectionUnknown) {
		t.Fatalf("err = %v, want unknown section", err)
	}
}

func TestNewArticleRejectsUnknownStatus(t *testing.T) {
	_, err := NewArticle(Article{Title: "Статья", Status: "hidden"}, "author-1", now)
	if !errors.Is(err, errStatusUnknown) {
		t.Fatalf("err = %v, want unknown status", err)
	}
}

func TestApplyEditPublishesAndAdvancesVersion(t *testing.T) {
	article := created(t)
	published := now.Add(time.Hour)

	edit := draft()
	edit.Status = StatusPublished

	if err := article.ApplyEdit(edit, "author-2", 1, published); err != nil {
		t.Fatalf("ApplyEdit: %v", err)
	}

	if article.Status != StatusPublished {
		t.Errorf("status = %q, want %q", article.Status, StatusPublished)
	}
	if article.Envelope.Version != 2 {
		t.Errorf("version = %d, want 2", article.Envelope.Version)
	}
	if !article.Envelope.UpdatedAt.Equal(published) {
		t.Errorf("updatedAt = %v, want %v", article.Envelope.UpdatedAt, published)
	}
	if article.Envelope.CreatedAt != article.Envelope.CreatedAt.UTC() || !article.Envelope.CreatedAt.Equal(now) {
		t.Errorf("createdAt moved: %v", article.Envelope.CreatedAt)
	}
}

func TestApplyEditRejectsStaleVersion(t *testing.T) {
	article := created(t)

	err := article.ApplyEdit(draft(), "author-2", 7, now)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("err = %v, want version conflict", err)
	}
}

func TestApplyEditKeepsIdentityAndSlug(t *testing.T) {
	article := created(t)
	edit := draft()
	edit.Slug = ""

	if err := article.ApplyEdit(edit, "author-2", 1, now); err != nil {
		t.Fatalf("ApplyEdit: %v", err)
	}

	if article.Envelope.Id != "article-1" {
		t.Errorf("id = %q, want article-1", article.Envelope.Id)
	}
	if article.Slug != "kak-obyavit-nabeg" {
		t.Errorf("slug = %q, want the title-derived one", article.Slug)
	}
}

func TestSaveRevisionKeepsTheReplacedState(t *testing.T) {
	article := created(t)
	article.SaveRevision("первая правка", now)

	if len(article.Revisions) != 1 {
		t.Fatalf("revisions = %d, want 1", len(article.Revisions))
	}

	revision := article.Revisions[0]
	if revision.ArticleId != "article-1" {
		t.Errorf("articleId = %q, want article-1", revision.ArticleId)
	}
	if revision.Note != "первая правка" {
		t.Errorf("note = %q", revision.Note)
	}
	if revision.Article.Title != article.Title {
		t.Errorf("revision lost the article")
	}
	if revision.Article.Revisions != nil {
		t.Errorf("revision must not nest the history")
	}
}

func TestSaveRevisionIsBounded(t *testing.T) {
	article := created(t)

	for i := range MaxRevisions + 10 {
		article.SaveRevision("правка", now.Add(time.Duration(i)*time.Minute))
	}

	if len(article.Revisions) != MaxRevisions {
		t.Fatalf("revisions = %d, want %d", len(article.Revisions), MaxRevisions)
	}
}

func TestPruneRevisionsKeepsTheNewest(t *testing.T) {
	revisions := make([]Revision, 0, MaxRevisions+5)
	for i := range MaxRevisions + 5 {
		revisions = append(revisions, Revision{Id: string(rune('a' + i%26)), Note: "n"})
	}

	pruned := PruneRevisions(revisions)
	if len(pruned) != MaxRevisions {
		t.Fatalf("len = %d, want %d", len(pruned), MaxRevisions)
	}
	if pruned[0].Id != revisions[0].Id {
		t.Errorf("the newest revision was dropped")
	}
}

func TestRestoreBringsBackTheSavedState(t *testing.T) {
	article := created(t)
	original := article.Title
	article.SaveRevision("перед правкой", now)

	edit := draft()
	edit.Title = "Совсем другое"
	if err := article.ApplyEdit(edit, "author-2", 1, now); err != nil {
		t.Fatalf("ApplyEdit: %v", err)
	}

	if err := article.Restore(article.Revisions[0], "author-2", now); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if article.Title != original {
		t.Errorf("title = %q, want %q", article.Title, original)
	}
	if article.Envelope.Version != 3 {
		t.Errorf("version = %d, want 3", article.Envelope.Version)
	}
	if len(article.Revisions) < 2 {
		t.Errorf("restore must keep the replaced state as a revision, got %d", len(article.Revisions))
	}
}

func TestRestoreRejectsForeignRevision(t *testing.T) {
	article := created(t)
	foreign := Revision{ArticleId: "other-article", Article: draft()}

	if err := article.Restore(foreign, "author-1", now); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("err = %v, want revision not found", err)
	}
}
