package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/verification.v1.VerificationService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "Approve"): interceptor.Scope("user:verify"),
		interceptor.Method(srvName + "List"):    interceptor.Scope("user:verify"),
		interceptor.Method(srvName + "Reject"):  interceptor.Scope("user:verify"),

		// Self-service: Submit and Details act on the JWT subject's own
		// application.
		interceptor.Method(srvName + "Submit"):  interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "Details"): interceptor.ScopeAuthenticated,

		// VerificationStatus takes a user_id from the request and does not
		// compare it to the JWT subject, so it discloses another player's
		// application state. Left authenticated-only to keep this change
		// behaviour-preserving; the missing ownership check is tracked
		// separately.
		interceptor.Method(srvName + "VerificationStatus"): interceptor.ScopeAuthenticated,
	}
}
