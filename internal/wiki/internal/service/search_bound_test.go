package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"github.com/lasthearth/vsservice/internal/wiki/internal/service"
	"github.com/lasthearth/vsservice/internal/wiki/internal/service/sermapper"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
)

// recordingRepo wraps the shared fake and keeps every filter the service
// forwarded, so a test can tell a search the service refused from one it passed
// to the query, and can read the term that arrived.
type recordingRepo struct {
	*fakeRepo
	lists      int
	lastSearch string
}

func (r *recordingRepo) List(
	ctx context.Context,
	filter model.Filter,
	next string,
	limit int64,
) (*model.Page, error) {
	r.lists++
	r.lastSearch = filter.Search

	return r.fakeRepo.List(ctx, filter, next, limit)
}

// newSearchService builds the service over a recordingRepo.
func newSearchService(t *testing.T) (*service.Service, *recordingRepo) {
	t.Helper()

	cfg := zap.NewProductionConfig()
	l, err := logger.New(&cfg)
	if err != nil {
		t.Fatal(err)
	}

	repo := &recordingRepo{fakeRepo: newFakeRepo()}

	return service.New(service.Opts{
		Logger:   l,
		Repo:     repo,
		Mapper:   &sermapper.MapperImpl{},
		Notifier: &fakeNotifier{},
	}), repo
}

// listRequest is a public list call carrying one search term.
func listRequest(search string) *wikiv1.ListArticlesRequest {
	return &wikiv1.ListArticlesRequest{Search: search}
}

// wantInvalidArgument fails unless err is the domain error carrying
// codes.InvalidArgument, and returns its message.
func wantInvalidArgument(t *testing.T, err error) string {
	t.Helper()

	var domainErr *ierror.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("err = %v (%T), want an invalid-argument domain error", err, err)
	}
	if domainErr.Code != codes.InvalidArgument {
		t.Fatalf("code = %v, want %v", domainErr.Code, codes.InvalidArgument)
	}

	return domainErr.Message
}

// TestListArticlesRefusesAnOverLongSearch is the regression the audit named: the
// search term reaches a $regex, which doubles every metacharacter, so the term
// is bounded before the repository sees it. 65 runes is one over the bound.
func TestListArticlesRefusesAnOverLongSearch(t *testing.T) {
	svc, repo := newSearchService(t)

	got, err := svc.ListArticles(anonymous(), listRequest(strings.Repeat("а", 65)))
	if err == nil {
		t.Fatal("a 65 rune search was accepted")
	}
	msg := wantInvalidArgument(t, err)
	if !strings.Contains(msg, "64") {
		t.Errorf("message = %q, want it to name the bound", msg)
	}
	if got != nil {
		t.Errorf("response = %+v, want none", got)
	}
	if repo.lists != 0 {
		t.Errorf("the over-long search reached the repository %d times", repo.lists)
	}
}

// TestListArticlesRefusesAnOversizedBodyOnItsByteLength covers the pre-check: a
// body far over the limit is answered on its byte length, so the rune count is
// never computed over it and the term is never escaped. The answer is the same
// as for a term one rune over, which is the point - the cost is what differs.
func TestListArticlesRefusesAnOversizedBodyOnItsByteLength(t *testing.T) {
	svc, repo := newSearchService(t)

	huge := strings.Repeat("(a*)*b", 4<<20/6) // ~4 MiB of metacharacters

	if len(huge) < 1<<20 {
		t.Fatalf("test input is %d bytes, want at least 1 MiB", len(huge))
	}

	_, err := svc.ListArticles(anonymous(), listRequest(huge))
	wantInvalidArgument(t, err)

	if repo.lists != 0 {
		t.Errorf("the oversized search reached the repository %d times", repo.lists)
	}
}

// TestListArticlesKeepsTheLongestLegalSearch pins that the bound counts runes
// and not bytes: 64 four-byte runes are 256 bytes, the widest term the bound
// allows, and must still be searched for.
func TestListArticlesKeepsTheLongestLegalSearch(t *testing.T) {
	cases := map[string]string{
		"64 latin runes":     strings.Repeat("a", 64),
		"64 cyrillic runes":  strings.Repeat("а", 64),
		"64 four-byte runes": strings.Repeat("🙂", 64),
	}

	for name, search := range cases {
		t.Run(name, func(t *testing.T) {
			svc, repo := newSearchService(t)

			if _, err := svc.ListArticles(anonymous(), listRequest(search)); err != nil {
				t.Fatalf("a %d rune search was refused: %v", len([]rune(search)), err)
			}
			if repo.lists != 1 {
				t.Fatalf("the repository was called %d times, want 1", repo.lists)
			}
			if repo.lastSearch != search {
				t.Errorf("search = %q, want the term unchanged", repo.lastSearch)
			}
		})
	}
}

// TestListArticlesTrimsTheSearchBeforeItIsForwarded pins the trimming the bound
// applies, so a padded term searches for the term and not for the padding.
func TestListArticlesTrimsTheSearchBeforeItIsForwarded(t *testing.T) {
	svc, repo := newSearchService(t)

	if _, err := svc.ListArticles(anonymous(), listRequest("  осада \n")); err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if repo.lastSearch != "осада" {
		t.Errorf("search = %q, want %q", repo.lastSearch, "осада")
	}
}

// TestListArticlesSearchesWithoutATerm keeps the empty search working: the bound
// narrows an oversized term, it does not make a term mandatory.
func TestListArticlesSearchesWithoutATerm(t *testing.T) {
	svc, repo := newSearchService(t)

	if _, err := svc.ListArticles(anonymous(), listRequest("")); err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if repo.lists != 1 || repo.lastSearch != "" {
		t.Errorf("lists = %d, search = %q, want 1 and an empty term", repo.lists, repo.lastSearch)
	}
}
