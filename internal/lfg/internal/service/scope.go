package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope: posting, responding, closing, renewing and reading a contact need a
// signed-in player; the author checks live on the model. ListPosts and
// GetPost are public (see interceptor publicMethods).
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/lfg.v1.LfgService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "CreatePost"):     interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "RespondToPost"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ClosePost"):      interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "RenewPost"):      interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetPostContact"): interceptor.ScopeAuthenticated,
	}
}
