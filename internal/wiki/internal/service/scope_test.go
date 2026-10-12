package service

import (
	"testing"

	wikiv1 "github.com/lasthearth/vsservice/gen/wiki/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Every WikiService method must be classified: either public in the auth
// matcher's list of readable methods, or present in this Scoper's table.
// Startup enforces the same rule for the whole server, and an omission would
// otherwise be reachable by any authenticated caller.
func TestEveryMethodClassified(t *testing.T) {
	cfg := zap.NewProductionConfig()
	l, err := logger.New(&cfg)
	if err != nil {
		t.Fatal(err)
	}

	s := &Service{}
	srv := grpc.NewServer()
	wikiv1.RegisterWikiServiceServer(srv, s)

	auth := interceptor.NewAuth(interceptor.Opts{Log: l, Scopers: []interceptor.Scoper{s}})

	// The readable methods are public in the matcher, so the coverage check
	// must not complain about them; the write paths are in the table below.
	if err := auth.VerifyCoverage(srv); err != nil {
		t.Fatal(err)
	}
}

// The roles are the point of this table: a player may propose, only an editor
// may write a page directly or decide a proposal.
func TestScopesSeparatePlayersFromEditors(t *testing.T) {
	table := (&Service{}).Scope()

	want := map[string]interceptor.Scope{
		"/wiki.v1.WikiService/CreateArticle":   "wiki:write",
		"/wiki.v1.WikiService/UpdateArticle":   "wiki:write",
		"/wiki.v1.WikiService/DeleteArticle":   "wiki:write",
		"/wiki.v1.WikiService/ListRevisions":   "wiki:write",
		"/wiki.v1.WikiService/RestoreRevision": "wiki:write",

		"/wiki.v1.WikiService/SubmitSuggestion":  interceptor.ScopeAuthenticated,
		"/wiki.v1.WikiService/ListSuggestions":   interceptor.ScopeAuthenticated,
		"/wiki.v1.WikiService/ApproveSuggestion": "wiki:review",
		"/wiki.v1.WikiService/RejectSuggestion":  "wiki:review",
	}

	for method, scope := range want {
		got, ok := table[interceptor.Method(method)]
		if !ok {
			t.Errorf("%s is not classified", method)
			continue
		}
		if got != scope {
			t.Errorf("%s is %q, want %q", method, got, scope)
		}
	}

	// Reading is public, so it must not appear in the table at all: a scope
	// there would deny the visitor the wiki is written for.
	for _, method := range []string{
		"/wiki.v1.WikiService/GetArticle",
		"/wiki.v1.WikiService/GetArticleBySlug",
		"/wiki.v1.WikiService/ListArticles",
	} {
		if scope, ok := table[interceptor.Method(method)]; ok {
			t.Errorf("%s must stay public, but is scoped as %q", method, scope)
		}
	}
}
