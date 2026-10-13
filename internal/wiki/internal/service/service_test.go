package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/jwt"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"github.com/lasthearth/vsservice/internal/wiki/internal/service"
	"github.com/lasthearth/vsservice/internal/wiki/internal/service/sermapper"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

// errFakeWrite stands in for a storage failure on the article collection.
var errFakeWrite = errors.New("fake article write failed")

// fakeRepo is an in-memory stand-in for MongoDB: the service rules are what
// these tests exercise, not the driver.
type fakeRepo struct {
	articles    map[string]*model.Article
	suggestions map[string]*model.Suggestion
	saved       int
	removed     string
	nextId      int
	// lastExpected is the version the last Save was pinned to, so a test can
	// tell a guarded write from an unguarded one.
	lastExpected int64
	// failArticleWrite makes every article write fail, so a test can see what the
	// service does when the wiki side of an approval does not land.
	failArticleWrite bool
}

func (f *fakeRepo) CreateSuggestion(_ context.Context, suggestion *model.Suggestion) (*model.Suggestion, error) {
	stored := *suggestion
	// The real repository reads the document back after inserting it, which is
	// where the identifier comes from; the fake applies the same envelope
	// through the model, the way the repository does.
	id := "sug-" + string(rune('0'+len(f.suggestions)+1))
	stored.AdoptDocument(id, stored.CreatedAt)
	stored.AdoptEnvelope(model.Envelope{
		Id:        id,
		CreatedAt: stored.CreatedAt,
		Version:   1,
	})
	f.suggestions[id] = &stored

	return &stored, nil
}

func (f *fakeRepo) GetSuggestion(_ context.Context, id string) (*model.Suggestion, error) {
	suggestion, ok := f.suggestions[id]
	if !ok {
		return nil, model.ErrSuggestionNotFound
	}

	copied := *suggestion

	return &copied, nil
}

func (f *fakeRepo) ListSuggestions(
	_ context.Context,
	filter model.SuggestionFilter,
	_ string,
	_ int64,
) (*model.SuggestionPage, error) {
	page := &model.SuggestionPage{}
	for _, suggestion := range f.suggestions {
		if filter.AuthorName != "" && suggestion.AuthorName != filter.AuthorName {
			continue
		}
		if filter.Status != "" && suggestion.Status != filter.Status {
			continue
		}
		page.Suggestions = append(page.Suggestions, suggestion)
	}

	return page, nil
}

func (f *fakeRepo) CountPendingSuggestions(_ context.Context, authorName string) (int64, error) {
	var count int64
	for _, suggestion := range f.suggestions {
		if suggestion.AuthorName == authorName && suggestion.Status == model.SuggestionPending {
			count++
		}
	}

	return count, nil
}

func (f *fakeRepo) SaveSuggestion(_ context.Context, suggestion *model.Suggestion) (*model.Suggestion, error) {
	stored, ok := f.suggestions[suggestion.Envelope.Id]
	if !ok {
		return nil, model.ErrSuggestionNotFound
	}
	if !stored.IsPending() {
		return nil, model.ErrSuggestionReviewed
	}

	saved := *suggestion
	f.suggestions[saved.Envelope.Id] = &saved

	return &saved, nil
}

func (f *fakeRepo) SetAppliedSuggestionArticle(_ context.Context, id string, articleId string) error {
	stored, ok := f.suggestions[id]
	if !ok || stored.Status != model.SuggestionApproved {
		return model.ErrSuggestionNotFound
	}

	stored.SetAppliedArticle(articleId)

	return nil
}

func (f *fakeRepo) ReopenSuggestion(_ context.Context, id string) error {
	stored, ok := f.suggestions[id]
	if !ok {
		return model.ErrSuggestionNotFound
	}
	if stored.Status != model.SuggestionApproved {
		return nil
	}

	stored.Reopen()

	return nil
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		articles:    map[string]*model.Article{},
		suggestions: map[string]*model.Suggestion{},
		nextId:      1,
	}
}

func (f *fakeRepo) Create(_ context.Context, article *model.Article) (*model.Article, error) {
	if f.failArticleWrite {
		return nil, errFakeWrite
	}

	for _, existing := range f.articles {
		if existing.Slug == article.Slug {
			return nil, model.ErrSlugTaken
		}
	}

	stored := *article
	id := "id-" + string(rune('0'+f.nextId))
	stored.AdoptDocument(model.Envelope{
		Id:      id,
		Version: 1,
	})
	f.nextId++
	f.articles[id] = &stored

	return &stored, nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (*model.Article, error) {
	article, ok := f.articles[id]
	if !ok {
		return nil, model.ErrNotFound
	}

	copied := *article

	return &copied, nil
}

func (f *fakeRepo) GetBySlug(_ context.Context, slug string) (*model.Article, error) {
	for _, article := range f.articles {
		if article.Slug == slug {
			copied := *article

			return &copied, nil
		}
	}

	return nil, model.ErrNotFound
}

func (f *fakeRepo) ExistsSlug(_ context.Context, slug string, exceptId string) (bool, error) {
	for id, article := range f.articles {
		if article.Slug == slug && id != exceptId {
			return true, nil
		}
	}

	return false, nil
}

func (f *fakeRepo) List(_ context.Context, filter model.Filter, _ string, _ int64) (*model.Page, error) {
	page := &model.Page{}
	for _, article := range f.articles {
		// Mirror repository.listFilter: a status filter narrows the result, it
		// never widens it past what IncludeDrafts allows.
		if !filter.IncludeDrafts {
			if article.Status != model.StatusPublished {
				continue
			}
		} else if filter.Status != "" && article.Status != filter.Status {
			continue
		}
		page.Articles = append(page.Articles, article)
	}
	page.Total = int64(len(page.Articles))

	return page, nil
}

func (f *fakeRepo) Save(_ context.Context, article *model.Article, expectedVersion int64) (*model.Article, error) {
	f.lastExpected = expectedVersion

	if f.failArticleWrite {
		return nil, errFakeWrite
	}

	stored, ok := f.articles[article.Envelope.Id]
	if !ok {
		return nil, model.ErrNotFound
	}
	if expectedVersion != 0 && expectedVersion != stored.Envelope.Version {
		return nil, model.ErrVersionConflict
	}

	saved := *article
	saved.Envelope.Version = stored.Envelope.Version + 1
	f.articles[saved.Envelope.Id] = &saved
	f.saved++

	return &saved, nil
}

func (f *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := f.articles[id]; !ok {
		return model.ErrNotFound
	}
	delete(f.articles, id)
	f.removed = id

	return nil
}

// notifyCall is one notification the service sent.
type notifyCall struct {
	userId  string
	title   string
	message string
}

// fakeNotifier records what a player was told.
type fakeNotifier struct {
	calls []notifyCall
	// err makes every notification fail, so a test can see what a decision does
	// when the message does not get through.
	err error
}

func (n *fakeNotifier) Notify(_ context.Context, userId, title, message string) error {
	n.calls = append(n.calls, notifyCall{userId: userId, title: title, message: message})

	return n.err
}

func newService(t *testing.T) (*service.Service, *fakeRepo) {
	svc, repo, _ := newServiceWithNotifier(t)

	return svc, repo
}

func newServiceWithNotifier(t *testing.T) (*service.Service, *fakeRepo, *fakeNotifier) {
	t.Helper()

	cfg := zap.NewProductionConfig()
	l, err := logger.New(&cfg)
	if err != nil {
		t.Fatal(err)
	}

	repo := newFakeRepo()
	notifier := &fakeNotifier{}

	return service.New(service.Opts{
		Logger:   l,
		Repo:     repo,
		Mapper:   &sermapper.MapperImpl{},
		Notifier: notifier,
	}), repo, notifier
}

func anonymous() context.Context {
	return context.Background()
}

func authenticated() context.Context {
	return interceptor.ContextWithUserID(context.Background(), "author-1")
}

func createRequest(title string) *wikiv1.CreateArticleRequest {
	return &wikiv1.CreateArticleRequest{
		Article: &wikiv1.Article{Title: title, Section: model.SectionWar, Body: "## Шаги"},
	}
}

func TestCreateArticleRequiresAuthentication(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.CreateArticle(anonymous(), createRequest("Новая")); err == nil {
		t.Fatal("expected an error for an anonymous caller")
	}
}

func TestCreateArticleDerivesSlugAndDefaultsToDraft(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Как объявить набег"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	if created.GetSlug() != "kak-obyavit-nabeg" {
		t.Errorf("slug = %q", created.GetSlug())
	}
	if created.GetStatus() != wikiv1.ArticleStatus_ARTICLE_STATUS_DRAFT {
		t.Errorf("status = %v, want draft", created.GetStatus())
	}
	if created.GetId() == "" {
		t.Error("created article has no id")
	}
}

// TestCreateArticleRecordsTheAuthor covers the author that is fixed at creation:
// the page belongs to whoever created it, however it is edited later.
func TestCreateArticleRecordsTheAuthor(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Засады"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	if created.GetCreatedBy() != "author-1" {
		t.Errorf("created_by = %q, want the caller", created.GetCreatedBy())
	}
}

func TestCreateArticleRejectsTakenSlug(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.CreateArticle(authenticated(), createRequest("Поселения")); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if !errors.Is(err, model.ErrSlugTaken) {
		t.Fatalf("err = %v, want slug taken", err)
	}
}

func TestUpdateArticleSavesRevisionAndAdvancesVersion(t *testing.T) {
	svc, repo := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	updated, err := svc.UpdateArticle(authenticated(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      created.GetId(),
			Title:   "Поселения и роли",
			Section: model.SectionSettlements,
			Status:  wikiv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED,
		},
		ExpectedVersion: created.GetVersion(),
		Note:            "уточнил про роли",
	})
	if err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	if updated.GetTitle() != "Поселения и роли" {
		t.Errorf("title = %q", updated.GetTitle())
	}
	if updated.GetStatus() != wikiv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED {
		t.Errorf("status = %v, want published", updated.GetStatus())
	}
	if updated.GetVersion() != created.GetVersion()+1 {
		t.Errorf("version = %d, want %d", updated.GetVersion(), created.GetVersion()+1)
	}

	stored := repo.articles[created.GetId()]
	if len(stored.Revisions) != 1 {
		t.Fatalf("revisions = %d, want 1", len(stored.Revisions))
	}
	if stored.Revisions[0].Note != "уточнил про роли" {
		t.Errorf("note = %q", stored.Revisions[0].Note)
	}
	if stored.Revisions[0].Article.Title != "Поселения" {
		t.Errorf("revision kept %q, want the replaced state", stored.Revisions[0].Article.Title)
	}
}

func TestUpdateArticleRejectsStaleVersion(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	_, err = svc.UpdateArticle(authenticated(), &wikiv1.UpdateArticleRequest{
		Article:         &wikiv1.Article{Id: created.GetId(), Title: "Другое", Section: model.SectionWar},
		ExpectedVersion: created.GetVersion() + 5,
	})
	if !errors.Is(err, model.ErrVersionConflict) {
		t.Fatalf("err = %v, want version conflict", err)
	}
}

func TestUpdateArticleRejectsSlugOfAnotherArticle(t *testing.T) {
	svc, _ := newService(t)

	first, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.CreateArticle(authenticated(), createRequest("Набег")); err != nil {
		t.Fatalf("second: %v", err)
	}

	_, err = svc.UpdateArticle(authenticated(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      first.GetId(),
			Title:   "Поселения",
			Slug:    "nabeg",
			Section: model.SectionWar,
		},
		ExpectedVersion: first.GetVersion(),
	})
	if !errors.Is(err, model.ErrSlugTaken) {
		t.Fatalf("err = %v, want slug taken", err)
	}
}

func TestUpdateArticleKeepsItsOwnSlug(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	updated, err := svc.UpdateArticle(authenticated(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      created.GetId(),
			Title:   "Поселения и роли",
			Slug:    created.GetSlug(),
			Section: model.SectionSettlements,
		},
		ExpectedVersion: created.GetVersion(),
	})
	if err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	if updated.GetSlug() != created.GetSlug() {
		t.Errorf("slug = %q, want the unchanged %q", updated.GetSlug(), created.GetSlug())
	}
}

// TestUpdateArticleByAnotherUserKeepsTheAuthor covers the split between the
// author and the last editor: an edit moves author_name, never created_by.
func TestUpdateArticleByAnotherUserKeepsTheAuthor(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	editor := interceptor.ContextWithUserID(context.Background(), "editor-9")

	updated, err := svc.UpdateArticle(editor, &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      created.GetId(),
			Title:   "Поселения и роли",
			Section: model.SectionSettlements,
		},
		ExpectedVersion: created.GetVersion(),
	})
	if err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	if updated.GetCreatedBy() != "author-1" {
		t.Errorf("created_by = %q, want the unchanged author", updated.GetCreatedBy())
	}
	if updated.GetAuthorName() != "editor-9" {
		t.Errorf("author_name = %q, want the new editor", updated.GetAuthorName())
	}
}

func TestListArticlesHidesDraftsFromReaders(t *testing.T) {
	svc, _ := newService(t)

	draft, err := svc.CreateArticle(authenticated(), createRequest("Черновик"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	if _, err := svc.UpdateArticle(authenticated(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      draft.GetId(),
			Title:   "Опубликованное",
			Section: model.SectionWar,
			Status:  wikiv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED,
		},
		ExpectedVersion: draft.GetVersion(),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := svc.CreateArticle(authenticated(), createRequest("Ещё черновик")); err != nil {
		t.Fatalf("second draft: %v", err)
	}

	forReaders, err := svc.ListArticles(anonymous(), &wikiv1.ListArticlesRequest{})
	if err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if len(forReaders.GetArticles()) != 1 {
		t.Fatalf("reader sees %d articles, want 1 published", len(forReaders.GetArticles()))
	}
	if forReaders.GetArticles()[0].GetTitle() != "Опубликованное" {
		t.Errorf("reader sees %q", forReaders.GetArticles()[0].GetTitle())
	}
}

// TestListArticlesIgnoresADraftStatusFilterFromAReader covers the way in: the
// list is public, so asking for drafts by name must not return them.
func TestListArticlesIgnoresADraftStatusFilterFromAReader(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.CreateArticle(authenticated(), createRequest("Черновик")); err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	forReaders, err := svc.ListArticles(anonymous(), &wikiv1.ListArticlesRequest{
		Status: wikiv1.ArticleStatus_ARTICLE_STATUS_DRAFT,
	})
	if err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if len(forReaders.GetArticles()) != 0 {
		t.Errorf("reader asking for drafts sees %d articles, want 0", len(forReaders.GetArticles()))
	}
}

// TestGetArticleHidesADraftFromAReader covers the direct fetch: a draft is
// reported as missing until it is published.
func TestGetArticleHidesADraftFromAReader(t *testing.T) {
	svc, _ := newService(t)

	draft, err := svc.CreateArticle(authenticated(), createRequest("Черновик"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	if _, err := svc.GetArticle(anonymous(), &wikiv1.GetArticleRequest{Id: draft.GetId()}); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("GetArticle err = %v, want not found", err)
	}
	if _, err := svc.GetArticleBySlug(anonymous(), &wikiv1.GetArticleBySlugRequest{
		Slug: draft.GetSlug(),
	}); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("GetArticleBySlug err = %v, want not found", err)
	}

	publish, err := svc.UpdateArticle(authenticated(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      draft.GetId(),
			Title:   "Черновик",
			Section: model.SectionWar,
			Status:  wikiv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED,
		},
		ExpectedVersion: draft.GetVersion(),
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	got, err := svc.GetArticleBySlug(anonymous(), &wikiv1.GetArticleBySlugRequest{Slug: publish.GetSlug()})
	if err != nil {
		t.Fatalf("GetArticleBySlug after publish: %v", err)
	}
	if got.GetStatus() != wikiv1.ArticleStatus_ARTICLE_STATUS_PUBLISHED {
		t.Errorf("status = %v, want published", got.GetStatus())
	}
}

func TestDeleteArticleRemovesIt(t *testing.T) {
	svc, repo := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Временная"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	if _, err := svc.DeleteArticle(authenticated(), &wikiv1.DeleteArticleRequest{Id: created.GetId()}); err != nil {
		t.Fatalf("DeleteArticle: %v", err)
	}
	if repo.removed != created.GetId() {
		t.Errorf("removed = %q, want %q", repo.removed, created.GetId())
	}
}

func TestRestoreRevisionBringsBackTheOlderState(t *testing.T) {
	svc, repo := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	updated, err := svc.UpdateArticle(authenticated(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      created.GetId(),
			Title:   "Поселения и роли",
			Section: model.SectionSettlements,
		},
		ExpectedVersion: created.GetVersion(),
		Note:            "переписал",
	})
	if err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	revisions, err := svc.ListRevisions(authenticated(), &wikiv1.ListRevisionsRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions.GetRevisions()) != 1 {
		t.Fatalf("revisions = %d, want 1", len(revisions.GetRevisions()))
	}

	restored, err := svc.RestoreRevision(authenticated(), &wikiv1.RestoreRevisionRequest{
		Id:         created.GetId(),
		RevisionId: revisions.GetRevisions()[0].GetId(),
	})
	if err != nil {
		t.Fatalf("RestoreRevision: %v", err)
	}

	if restored.GetTitle() != "Поселения" {
		t.Errorf("title = %q, want the restored %q", restored.GetTitle(), "Поселения")
	}
	if restored.GetVersion() <= updated.GetVersion() {
		t.Errorf("version = %d, want more than %d", restored.GetVersion(), updated.GetVersion())
	}
	if repo.lastExpected == 0 {
		t.Error("restore saved without a version guard, so a concurrent edit would be replaced silently")
	}
}

// TestRestoreRevisionKeepsTheAuthor covers the restore path: bringing an older
// state back is an edit like any other, so the author stays the one on record.
func TestRestoreRevisionKeepsTheAuthor(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	editor := interceptor.ContextWithUserID(context.Background(), "editor-9")

	if _, err := svc.UpdateArticle(editor, &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      created.GetId(),
			Title:   "Поселения и роли",
			Section: model.SectionSettlements,
		},
		ExpectedVersion: created.GetVersion(),
	}); err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	revisions, err := svc.ListRevisions(editor, &wikiv1.ListRevisionsRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(revisions.GetRevisions()) != 1 {
		t.Fatalf("revisions = %d, want 1", len(revisions.GetRevisions()))
	}

	restored, err := svc.RestoreRevision(editor, &wikiv1.RestoreRevisionRequest{
		Id:         created.GetId(),
		RevisionId: revisions.GetRevisions()[0].GetId(),
	})
	if err != nil {
		t.Fatalf("RestoreRevision: %v", err)
	}

	if restored.GetCreatedBy() != "author-1" {
		t.Errorf("created_by = %q, want the unchanged author", restored.GetCreatedBy())
	}
}

func TestRestoreRevisionRejectsUnknownRevision(t *testing.T) {
	svc, _ := newService(t)

	created, err := svc.CreateArticle(authenticated(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	_, err = svc.RestoreRevision(authenticated(), &wikiv1.RestoreRevisionRequest{
		Id:         created.GetId(),
		RevisionId: "nope",
	})
	if !errors.Is(err, model.ErrRevisionNotFound) {
		t.Fatalf("err = %v, want revision not found", err)
	}
}

func TestUpdateArticleRequiresId(t *testing.T) {
	svc, _ := newService(t)

	_, err := svc.UpdateArticle(authenticated(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{Title: "Без id"},
	})
	if err == nil {
		t.Fatal("expected an error for a missing id")
	}
}

func reviewer() context.Context {
	ctx := interceptor.ContextWithUserID(context.Background(), "editor-1")

	return interceptor.ContextWithClaims(ctx, &jwt.Claims{Scope: "wiki:write wiki:review"})
}

// editRequest proposes a change to an existing article, saying which version of
// it the author was looking at.
func editRequest(title string, articleId string, baseVersion int64) *wikiv1.SubmitSuggestionRequest {
	return &wikiv1.SubmitSuggestionRequest{
		Suggestion: &wikiv1.Suggestion{
			ArticleId:   articleId,
			BaseVersion: baseVersion,
			Proposal:    &wikiv1.Article{Title: title, Section: model.SectionWar, Body: "## Правка игрока"},
			Note:        "нашёл неточность",
		},
	}
}

// rawSuggestion is model.Suggestion with its fields writable. It is how a test
// builds a proposal from before base versions were kept, which no code path
// produces any more.
type rawSuggestion model.Suggestion

func legacyEditProposal(suggestion *model.Suggestion) *model.Suggestion {
	raw := rawSuggestion(*suggestion)
	raw.BaseVersion = 0
	raw.Base = nil

	legacy := model.Suggestion(raw)

	return &legacy
}

func suggestionRequest(title string, articleId ...string) *wikiv1.SubmitSuggestionRequest {
	target := ""
	if len(articleId) > 0 {
		target = articleId[0]
	}

	return &wikiv1.SubmitSuggestionRequest{
		Suggestion: &wikiv1.Suggestion{
			ArticleId: target,
			Proposal:  &wikiv1.Article{Title: title, Section: model.SectionWar, Body: "## Черновик игрока"},
			Note:      "написал статью",
		},
	}
}

func TestSubmitSuggestionStoresWithoutPublishing(t *testing.T) {
	svc, repo := newService(t)

	created, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if created.GetStatus() != wikiv1.SuggestionStatus_SUGGESTION_STATUS_PENDING {
		t.Errorf("status = %v, want pending", created.GetStatus())
	}
	if created.GetKind() != wikiv1.SuggestionKind_SUGGESTION_KIND_NEW_ARTICLE {
		t.Errorf("kind = %v, want a new article", created.GetKind())
	}
	if created.GetSlug() != "zasady" {
		t.Errorf("slug = %q", created.GetSlug())
	}

	// Nothing may reach the public list before an editor decides.
	listed, err := svc.ListArticles(anonymous(), &wikiv1.ListArticlesRequest{})
	if err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if len(listed.GetArticles()) != 0 {
		t.Fatalf("a proposal leaked into the published list: %d articles", len(listed.GetArticles()))
	}
	if len(repo.articles) != 0 {
		t.Fatalf("a proposal created an article: %d", len(repo.articles))
	}
}

func TestSubmitSuggestionRequiresAuthentication(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.SubmitSuggestion(anonymous(), suggestionRequest("Засады")); err == nil {
		t.Fatal("expected an error for an anonymous caller")
	}
}

func TestSubmitSuggestionCountsAgainstTheLimit(t *testing.T) {
	svc, _ := newService(t)

	for i := range model.MaxPendingSuggestions {
		if _, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады "+string(rune('a'+i)))); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	_, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Ещё одна"))
	if !errors.Is(err, model.ErrTooManySuggestions) {
		t.Fatalf("err = %v, want too many suggestions", err)
	}
}

func TestSubmitSuggestionForAnEditNeedsTheArticle(t *testing.T) {
	svc, _ := newService(t)

	_, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Правка", "no-such-article"))
	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestApproveSuggestionPublishesANewArticle(t *testing.T) {
	svc, _ := newService(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	approved, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{Id: suggestion.GetId()})
	if err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}
	if approved.GetStatus() != wikiv1.SuggestionStatus_SUGGESTION_STATUS_APPROVED {
		t.Errorf("status = %v, want approved", approved.GetStatus())
	}
	if approved.GetAppliedArticleId() == "" {
		t.Error("approved proposal has no article")
	}

	listed, err := svc.ListArticles(anonymous(), &wikiv1.ListArticlesRequest{})
	if err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if len(listed.GetArticles()) != 1 {
		t.Fatalf("approved proposal produced %d published articles, want 1", len(listed.GetArticles()))
	}
	if listed.GetArticles()[0].GetTitle() != "Засады" {
		t.Errorf("published %q", listed.GetArticles()[0].GetTitle())
	}
}

func TestApproveSuggestionAppliesAnEditAsARevision(t *testing.T) {
	svc, repo := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Поселения и роли", article.GetId()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	// The published article must not change while the proposal is reviewed.
	stored := repo.articles[article.GetId()]
	if stored.Title != "Поселения" {
		t.Fatalf("the proposal changed the article before review: %q", stored.Title)
	}

	if _, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{Id: suggestion.GetId()}); err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	stored = repo.articles[article.GetId()]
	if stored.Title != "Поселения и роли" {
		t.Errorf("title = %q, want the proposal applied", stored.Title)
	}
	if len(stored.Revisions) == 0 {
		t.Error("an approved edit must leave a revision behind")
	}
}

// TestApproveSuggestionOfANewArticleCreditsItsAuthor covers who an approved
// proposal belongs to: the player who wrote it, not the editor who let it in.
func TestApproveSuggestionOfANewArticleCreditsItsAuthor(t *testing.T) {
	svc, _ := newService(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	approved, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{Id: suggestion.GetId()})
	if err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	article, err := svc.GetArticle(anonymous(), &wikiv1.GetArticleRequest{Id: approved.GetAppliedArticleId()})
	if err != nil {
		t.Fatalf("GetArticle: %v", err)
	}

	if article.GetCreatedBy() != "author-1" {
		t.Errorf("created_by = %q, want the proposal's author", article.GetCreatedBy())
	}
	if article.GetAuthorName() != "author-1" {
		t.Errorf("author_name = %q, want the proposal's author, not the reviewer", article.GetAuthorName())
	}
}

// TestApproveSuggestionOfAnEditCreditsTheProposalWithTheLastEditor covers the
// same rule on a page that already exists: approval moves author_name to the
// proposer and leaves created_by where it was.
func TestApproveSuggestionOfAnEditCreditsTheProposalWithTheLastEditor(t *testing.T) {
	svc, _ := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Поселения и роли", article.GetId()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if _, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	}); err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	updated, err := svc.GetArticle(reviewer(), &wikiv1.GetArticleRequest{Id: article.GetId()})
	if err != nil {
		t.Fatalf("GetArticle: %v", err)
	}

	if updated.GetCreatedBy() != "editor-1" {
		t.Errorf("created_by = %q, want the unchanged author", updated.GetCreatedBy())
	}
	if updated.GetAuthorName() != "author-1" {
		t.Errorf("author_name = %q, want the proposal's author", updated.GetAuthorName())
	}
}

// TestSubmitSuggestionKeepsTheArticleItWasWrittenAgainst covers what an edit
// proposal remembers: the version the author read and the text they saw, so a
// reviewer can tell what approving it would replace.
func TestSubmitSuggestionKeepsTheArticleItWasWrittenAgainst(t *testing.T) {
	svc, repo := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения и роли", article.GetId(), article.GetVersion()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if suggestion.GetBaseVersion() != article.GetVersion() {
		t.Errorf("base_version = %d, want %d", suggestion.GetBaseVersion(), article.GetVersion())
	}
	if suggestion.GetBase().GetTitle() != "Поселения" {
		t.Errorf("base title = %q, want the article as the author saw it", suggestion.GetBase().GetTitle())
	}

	// The base is stored whole, so the history it carries is checked on the
	// stored proposal: repeating it here would grow the document by every
	// revision of the article.
	stored := repo.suggestions[suggestion.GetId()]
	if stored.Base == nil {
		t.Fatal("the stored proposal kept no base")
	}
	if len(stored.Base.Revisions) != 0 {
		t.Errorf("the stored base carries %d revisions, want none", len(stored.Base.Revisions))
	}
}

// TestSubmitSuggestionRefusesABaseVersionTheArticleHasLeft covers the scenario
// the base version exists for: the author read v3, an editor saved v4, and the
// proposal is refused at the door instead of queueing a silent overwrite.
func TestSubmitSuggestionRefusesABaseVersionTheArticleHasLeft(t *testing.T) {
	svc, repo := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	if _, err := svc.UpdateArticle(reviewer(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      article.GetId(),
			Title:   "Поселения и роли",
			Section: model.SectionSettlements,
		},
		ExpectedVersion: article.GetVersion(),
	}); err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	_, err = svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения, роли и налоги", article.GetId(), article.GetVersion()))
	if !errors.Is(err, model.ErrSuggestionOutdated) {
		t.Fatalf("err = %v, want the outdated base version reported", err)
	}
	if len(repo.suggestions) != 0 {
		t.Errorf("a refused proposal was stored: %d", len(repo.suggestions))
	}
}

// TestSubmitSuggestionTakesTheCurrentVersionWhenNoneIsGiven covers a client
// that does not name a version: the proposal is pinned to what storage holds.
func TestSubmitSuggestionTakesTheCurrentVersionWhenNoneIsGiven(t *testing.T) {
	svc, _ := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения и роли", article.GetId(), 0))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if suggestion.GetBaseVersion() != article.GetVersion() {
		t.Errorf("base_version = %d, want the stored %d", suggestion.GetBaseVersion(), article.GetVersion())
	}
}

func TestApproveSuggestionAppliesAnEditWrittenAgainstTheCurrentVersion(t *testing.T) {
	svc, repo := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения и роли", article.GetId(), article.GetVersion()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if _, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	}); err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	if stored := repo.articles[article.GetId()]; stored.Title != "Поселения и роли" {
		t.Errorf("title = %q, want the proposal applied", stored.Title)
	}
}

// TestApproveSuggestionRefusesAnEditTheArticleMovedPast covers the other half:
// the proposal is accepted, the page moves on before review, and the approval
// is refused with ABORTED — the author's text never replaces the later save.
func TestApproveSuggestionRefusesAnEditTheArticleMovedPast(t *testing.T) {
	svc, repo := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения и роли", article.GetId(), article.GetVersion()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	// An editor saves the page while the proposal waits for review.
	if _, err := svc.UpdateArticle(reviewer(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      article.GetId(),
			Title:   "Поселения, роли и налоги",
			Section: model.SectionSettlements,
		},
		ExpectedVersion: article.GetVersion(),
	}); err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	_, err = svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{Id: suggestion.GetId()})
	if !errors.Is(err, model.ErrSuggestionStale) {
		t.Fatalf("err = %v, want the proposal reported as stale", err)
	}

	// ABORTED, not FAILED_PRECONDITION: the reviewer can retry with allow_stale,
	// while a proposal that was already decided cannot be retried at all. Over
	// REST the codes answer differently (409 vs 400), so a client can act on
	// the difference.
	var domainErr *ierror.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != codes.Aborted {
		t.Errorf("err = %v, want ABORTED", err)
	}

	if stored := repo.articles[article.GetId()]; stored.Title != "Поселения, роли и налоги" {
		t.Errorf("title = %q, want the later save kept", stored.Title)
	}

	queue, err := svc.ListSuggestions(reviewer(), &wikiv1.ListSuggestionsRequest{})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(queue.GetSuggestions()) != 1 ||
		queue.GetSuggestions()[0].GetStatus() != wikiv1.SuggestionStatus_SUGGESTION_STATUS_PENDING {
		t.Error("a refused approval took the proposal out of the queue")
	}
}

// TestApproveSuggestionAppliesAStaleEditWhenTheReviewerConfirms covers
// allow_stale: the reviewer has seen what would be replaced, so the proposal is
// applied and the state it replaced is kept as a revision.
func TestApproveSuggestionAppliesAStaleEditWhenTheReviewerConfirms(t *testing.T) {
	svc, repo := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения и роли", article.GetId(), article.GetVersion()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if _, err := svc.UpdateArticle(reviewer(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      article.GetId(),
			Title:   "Поселения, роли и налоги",
			Section: model.SectionSettlements,
		},
		ExpectedVersion: article.GetVersion(),
	}); err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	if _, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id:         suggestion.GetId(),
		AllowStale: true,
	}); err != nil {
		t.Fatalf("ApproveSuggestion with allow_stale: %v", err)
	}

	stored := repo.articles[article.GetId()]
	if stored.Title != "Поселения и роли" {
		t.Errorf("title = %q, want the proposal applied", stored.Title)
	}
	if len(stored.Revisions) == 0 {
		t.Fatal("an approved edit must leave a revision behind")
	}
	if stored.Revisions[0].Article.Title != "Поселения, роли и налоги" {
		t.Errorf("revision kept %q, want the state the approval replaced",
			stored.Revisions[0].Article.Title)
	}
}

// TestApproveSuggestionOfAProposalWithoutABaseVersion covers the queue that
// predates the field: nothing recorded which version it was written against, so
// it cannot be checked and only the reviewer's confirmation lets it through.
func TestApproveSuggestionOfAProposalWithoutABaseVersion(t *testing.T) {
	svc, repo := newService(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	submitted, err := svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения и роли", article.GetId(), article.GetVersion()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	repo.suggestions[submitted.GetId()] = legacyEditProposal(repo.suggestions[submitted.GetId()])

	_, err = svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{Id: submitted.GetId()})
	if !errors.Is(err, model.ErrSuggestionStale) {
		t.Fatalf("err = %v, want a proposal without a base version to count as stale", err)
	}
	if stored := repo.articles[article.GetId()]; stored.Title != "Поселения" {
		t.Errorf("title = %q, want the article untouched", stored.Title)
	}

	approved, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id:         submitted.GetId(),
		AllowStale: true,
	})
	if err != nil {
		t.Fatalf("ApproveSuggestion with allow_stale: %v", err)
	}
	if approved.GetAppliedArticleId() == "" {
		t.Error("the confirmed proposal produced no article")
	}
	if stored := repo.articles[article.GetId()]; stored.Title != "Поселения и роли" {
		t.Errorf("title = %q, want the proposal applied after the confirmation", stored.Title)
	}
}

// TestApproveSuggestionOfANewArticleIgnoresAllowStale covers the flag on the
// path where there is nothing to be stale against.
func TestApproveSuggestionOfANewArticleIgnoresAllowStale(t *testing.T) {
	svc, _ := newService(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	approved, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id:         suggestion.GetId(),
		AllowStale: true,
	})
	if err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	article, err := svc.GetArticle(anonymous(), &wikiv1.GetArticleRequest{
		Id: approved.GetAppliedArticleId(),
	})
	if err != nil {
		t.Fatalf("GetArticle: %v", err)
	}
	if article.GetTitle() != "Засады" {
		t.Errorf("title = %q, want the proposal published", article.GetTitle())
	}
}

// TestApproveSuggestionNotifiesTheAuthorOfANewArticle covers the message a
// player gets when the page they proposed is published.
func TestApproveSuggestionNotifiesTheAuthorOfANewArticle(t *testing.T) {
	svc, _, notifier := newServiceWithNotifier(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if _, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	}); err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	if len(notifier.calls) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifier.calls))
	}
	call := notifier.calls[0]
	if call.userId != "author-1" {
		t.Errorf("notified %q, want the author", call.userId)
	}
	if call.title != "Статья опубликована" {
		t.Errorf("title = %q", call.title)
	}
	if !strings.Contains(call.message, "https://lasthearth.ru/wiki/zasady") {
		t.Errorf("message = %q, want the address of the published page", call.message)
	}
}

// TestApproveSuggestionNotifiesTheAuthorOfAnEdit covers the other approval. The
// address is taken from the article the approval produced, because an edit is a
// save like any other and may have moved the page.
func TestApproveSuggestionNotifiesTheAuthorOfAnEdit(t *testing.T) {
	svc, repo, notifier := newServiceWithNotifier(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения и роли", article.GetId(), article.GetVersion()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if _, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	}); err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	if len(notifier.calls) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifier.calls))
	}
	call := notifier.calls[0]
	if call.userId != "author-1" {
		t.Errorf("notified %q, want the author", call.userId)
	}
	if call.title != "Правка принята" {
		t.Errorf("title = %q", call.title)
	}
	applied := repo.articles[article.GetId()]
	if !strings.Contains(call.message, "https://lasthearth.ru/wiki/"+applied.Slug) {
		t.Errorf("message = %q, want the address of the applied article", call.message)
	}
}

func TestRejectSuggestionNotifiesTheAuthorOfTheReason(t *testing.T) {
	svc, _, notifier := newServiceWithNotifier(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if _, err := svc.RejectSuggestion(reviewer(), &wikiv1.RejectSuggestionRequest{
		Id:     suggestion.GetId(),
		Reason: "не по теме",
	}); err != nil {
		t.Fatalf("RejectSuggestion: %v", err)
	}

	if len(notifier.calls) != 1 {
		t.Fatalf("notifications = %d, want 1", len(notifier.calls))
	}
	call := notifier.calls[0]
	if call.userId != "author-1" {
		t.Errorf("notified %q, want the author", call.userId)
	}
	if call.title != "Предложение отклонено" {
		t.Errorf("title = %q", call.title)
	}
	if !strings.Contains(call.message, "Засады") {
		t.Errorf("message = %q, want the page named in it", call.message)
	}
	if !strings.Contains(call.message, "не по теме") {
		t.Errorf("message = %q, want the reviewer's reason in it", call.message)
	}
}

// TestNoNotificationWhenTheDecisionIsNotStored covers the order of the two
// writes: an approval the article refuses leaves the proposal undecided, so the
// author must not be told it was applied.
func TestNoNotificationWhenTheDecisionIsNotStored(t *testing.T) {
	svc, _, notifier := newServiceWithNotifier(t)

	article, err := svc.CreateArticle(reviewer(), createRequest("Поселения"))
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}

	suggestion, err := svc.SubmitSuggestion(authenticated(),
		editRequest("Поселения и роли", article.GetId(), article.GetVersion()))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	// An editor saves the page, so the approval is refused as stale.
	if _, err := svc.UpdateArticle(reviewer(), &wikiv1.UpdateArticleRequest{
		Article: &wikiv1.Article{
			Id:      article.GetId(),
			Title:   "Поселения, роли и налоги",
			Section: model.SectionSettlements,
		},
		ExpectedVersion: article.GetVersion(),
	}); err != nil {
		t.Fatalf("UpdateArticle: %v", err)
	}

	if _, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	}); !errors.Is(err, model.ErrSuggestionStale) {
		t.Fatalf("err = %v, want the proposal refused as stale", err)
	}

	if len(notifier.calls) != 0 {
		t.Errorf("notifications = %d, want none for a decision that did not land", len(notifier.calls))
	}
}

// TestNotifierFailureLeavesTheDecisionStored covers the other direction: a
// message that cannot be sent is a warning, never a failed decision.
func TestNotifierFailureLeavesTheDecisionStored(t *testing.T) {
	svc, repo, notifier := newServiceWithNotifier(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	notifier.err = errors.New("notification store is down")

	approved, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	})
	if err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	if len(notifier.calls) != 1 {
		t.Fatalf("notifications = %d, want the attempt recorded", len(notifier.calls))
	}
	if approved.GetStatus() != wikiv1.SuggestionStatus_SUGGESTION_STATUS_APPROVED {
		t.Errorf("status = %v, want the decision kept", approved.GetStatus())
	}
	if approved.GetAppliedArticleId() == "" {
		t.Error("the decision was kept but produced no article")
	}
	if len(repo.articles) == 0 {
		t.Error("a failed notification rolled the article back")
	}
}

func TestReviewingTwiceIsRefused(t *testing.T) {
	svc, _ := newService(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	if _, err := svc.RejectSuggestion(reviewer(), &wikiv1.RejectSuggestionRequest{
		Id: suggestion.GetId(), Reason: "не по теме",
	}); err != nil {
		t.Fatalf("RejectSuggestion: %v", err)
	}

	_, err = svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{Id: suggestion.GetId()})
	if !errors.Is(err, model.ErrSuggestionReviewed) {
		t.Fatalf("err = %v, want already reviewed", err)
	}
}

func TestRejectSuggestionRequiresAReason(t *testing.T) {
	svc, _ := newService(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	_, err = svc.RejectSuggestion(reviewer(), &wikiv1.RejectSuggestionRequest{Id: suggestion.GetId()})
	if !errors.Is(err, model.ErrReasonRequired) {
		t.Fatalf("err = %v, want a required reason", err)
	}
}

func TestListSuggestionsShowsAuthorsOnlyTheirOwn(t *testing.T) {
	svc, _ := newService(t)

	if _, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады")); err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	own, err := svc.ListSuggestions(authenticated(), &wikiv1.ListSuggestionsRequest{})
	if err != nil {
		t.Fatalf("ListSuggestions as an author: %v", err)
	}
	if len(own.GetSuggestions()) != 1 {
		t.Fatalf("author sees %d proposals, want 1", len(own.GetSuggestions()))
	}

	other := interceptor.ContextWithUserID(context.Background(), "player-2")
	others, err := svc.ListSuggestions(other, &wikiv1.ListSuggestionsRequest{})
	if err != nil {
		t.Fatalf("ListSuggestions as another player: %v", err)
	}
	if len(others.GetSuggestions()) != 0 {
		t.Fatalf("a player sees someone else's proposals: %d", len(others.GetSuggestions()))
	}

	queue, err := svc.ListSuggestions(reviewer(), &wikiv1.ListSuggestionsRequest{})
	if err != nil {
		t.Fatalf("ListSuggestions as a reviewer: %v", err)
	}
	if len(queue.GetSuggestions()) != 1 {
		t.Fatalf("reviewer sees %d proposals, want the whole queue", len(queue.GetSuggestions()))
	}
}

// TestDecidingRefusesTheAuthorOfTheProposal covers the split between proposing
// and deciding: one person must not write a page through the review queue, not
// even when they hold the review scope.
func TestDecidingRefusesTheAuthorOfTheProposal(t *testing.T) {
	svc, repo := newService(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	author := interceptor.ContextWithClaims(authenticated(), &jwt.Claims{Scope: "wiki:write wiki:review"})

	if _, err := svc.ApproveSuggestion(author, &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	}); !errors.Is(err, model.ErrSelfReview) {
		t.Fatalf("ApproveSuggestion err = %v, want the author refused", err)
	}
	if len(repo.articles) != 0 {
		t.Errorf("a refused approval wrote %d articles", len(repo.articles))
	}

	if _, err := svc.RejectSuggestion(author, &wikiv1.RejectSuggestionRequest{
		Id: suggestion.GetId(), Reason: "сам себя",
	}); !errors.Is(err, model.ErrSelfReview) {
		t.Errorf("RejectSuggestion err = %v, want the author refused", err)
	}

	queue, err := svc.ListSuggestions(reviewer(), &wikiv1.ListSuggestionsRequest{})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(queue.GetSuggestions()) != 1 ||
		queue.GetSuggestions()[0].GetStatus() != wikiv1.SuggestionStatus_SUGGESTION_STATUS_PENDING {
		t.Error("a refused decision took the proposal out of the queue")
	}
}

// TestApproveSuggestionGivesTheReservationBackOnAFailedApply covers the order of
// the two writes: the decision is reserved before the article is written, so an
// apply that fails must leave the proposal decidable instead of approved with
// nothing behind it.
func TestApproveSuggestionGivesTheReservationBackOnAFailedApply(t *testing.T) {
	svc, repo := newService(t)

	suggestion, err := svc.SubmitSuggestion(authenticated(), suggestionRequest("Засады"))
	if err != nil {
		t.Fatalf("SubmitSuggestion: %v", err)
	}

	repo.failArticleWrite = true
	if _, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	}); !errors.Is(err, errFakeWrite) {
		t.Fatalf("err = %v, want the failed article write reported", err)
	}

	queue, err := svc.ListSuggestions(reviewer(), &wikiv1.ListSuggestionsRequest{})
	if err != nil {
		t.Fatalf("ListSuggestions: %v", err)
	}
	if len(queue.GetSuggestions()) != 1 ||
		queue.GetSuggestions()[0].GetStatus() != wikiv1.SuggestionStatus_SUGGESTION_STATUS_PENDING {
		t.Fatalf("the proposal stayed decided after a failed apply: %d entries", len(queue.GetSuggestions()))
	}

	repo.failArticleWrite = false
	approved, err := svc.ApproveSuggestion(reviewer(), &wikiv1.ApproveSuggestionRequest{
		Id: suggestion.GetId(),
	})
	if err != nil {
		t.Fatalf("ApproveSuggestion after the reservation was given back: %v", err)
	}
	if approved.GetAppliedArticleId() == "" {
		t.Error("the approved proposal does not name the article it produced")
	}
}
