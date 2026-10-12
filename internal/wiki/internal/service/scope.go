package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope declares which methods need a token and which need an editor.
//
// Reading the wiki is public: a visitor without a token must see published
// pages, so the read methods are listed as public in the auth matcher instead
// of here. Everything that changes a page, or exposes its history, requires an
// editor's scope.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/wiki.v1.WikiService/"

	return map[interceptor.Method]interceptor.Scope{
		// Reading is public: the three read methods are listed as public in the
		// auth matcher, so they must NOT appear here. A scope in this table
		// would take precedence and deny the visitor the wiki exists for.

		// Writing a page directly, and deciding what other people proposed, is
		// the editor's job; players go through suggestions below.
		interceptor.Method(srvName + "CreateArticle"):   interceptor.Scope("wiki:write"),
		interceptor.Method(srvName + "UpdateArticle"):   interceptor.Scope("wiki:write"),
		interceptor.Method(srvName + "DeleteArticle"):   interceptor.Scope("wiki:write"),
		interceptor.Method(srvName + "ListRevisions"):   interceptor.Scope("wiki:write"),
		interceptor.Method(srvName + "RestoreRevision"): interceptor.Scope("wiki:write"),

		// Any signed-in player may propose an article or an edit to one, and may
		// read back their own proposals.
		interceptor.Method(srvName + "SubmitSuggestion"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ListSuggestions"):  interceptor.ScopeAuthenticated,

		// Deciding a proposal is reserved for editors.
		interceptor.Method(srvName + "ApproveSuggestion"): interceptor.Scope("wiki:review"),
		interceptor.Method(srvName + "RejectSuggestion"):  interceptor.Scope("wiki:review"),
	}
}
