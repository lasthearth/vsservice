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

		// VerificationStatus looks up by a user_id from the request and returns
		// a single status field. VerifyStatusByName is public and returns the
		// same field keyed by game name instead, so this discloses nothing that
		// endpoint does not already — only the lookup key differs.
		interceptor.Method(srvName + "VerificationStatus"): interceptor.ScopeAuthenticated,
	}
}
