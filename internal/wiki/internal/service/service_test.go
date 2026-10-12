package service_test

import (
	"context"
	"errors"
	"testing"

	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/pkg/jwt"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"github.com/lasthearth/vsservice/internal/wiki/internal/service"
	"github.com/lasthearth/vsservice/internal/wiki/internal/service/sermapper"
	"go.uber.org/zap"
)

// fakeRepo is an in-memory stand-in for MongoDB: the service rules are what
// these tests exercise, not the driver.
type fakeRepo struct {
	articles    map[string]*model.Article
	suggestions map[string]*model.Suggestion
	saved       int
	removed     string
	nextId      int
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

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		articles:    map[string]*model.Article{},
		suggestions: map[string]*model.Suggestion{},
		nextId:      1,
	}
}

func (f *fakeRepo) Create(_ context.Context, article *model.Article) (*model.Article, error) {
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
		if filter.Status != "" && article.Status != filter.Status {
			continue
		}
		if !filter.IncludeDrafts && article.Status != model.StatusPublished {
			continue
		}
		page.Articles = append(page.Articles, article)
	}
	page.Total = int64(len(page.Articles))

	return page, nil
}

func (f *fakeRepo) Save(_ context.Context, article *model.Article, expectedVersion int64) (*model.Article, error) {
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

func newService(t *testing.T) (*service.Service, *fakeRepo) {
	t.Helper()

	cfg := zap.NewProductionConfig()
	l, err := logger.New(&cfg)
	if err != nil {
		t.Fatal(err)
	}

	repo := newFakeRepo()

	return service.New(service.Opts{
		Logger: l,
		Repo:   repo,
		Mapper: &sermapper.MapperImpl{},
	}), repo
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
	svc, _ := newService(t)

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
