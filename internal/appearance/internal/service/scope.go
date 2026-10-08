package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope: changing the look needs a signed-in player. ListAppearances is
// public (see interceptor publicMethods).
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/appearance.v1.AppearanceService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "UpdateMyAppearance"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ResetMyAppearance"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetMyStanding"):      interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "BuyBanner"):          interceptor.ScopeAuthenticated,
	}
}
