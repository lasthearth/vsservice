package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope implements interceptor.Scoper.
//
// Every method is self-service and carries no scope: the referrer and the
// referee are both the JWT subject. They are listed rather than omitted so a
// future method cannot become reachable by simply not appearing here — see
// interceptor.Auth.uncoveredMethods.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/referral.v1.ReferralService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "GetMyReferralCode"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "GetMyReferralStats"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "UseReferralCode"):    interceptor.ScopeAuthenticated,
	}
}
