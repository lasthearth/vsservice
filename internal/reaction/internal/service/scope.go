package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope: reacting needs only a signed-in player; ListReactions is public
// (see interceptor publicMethods).
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/reaction.v1.ReactionService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "ListMyReactions"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ToggleReaction"):  interceptor.ScopeAuthenticated,
	}
}
