package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/mail.v1.MailService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "ComposeMail"):    interceptor.Scope("mail:compose"),
		interceptor.Method(srvName + "ComposeKitMail"): interceptor.Scope("mail:compose"),
		interceptor.Method(srvName + "RevokeMail"):     interceptor.Scope("mail:revoke"),

		// Self-service: the JWT subject is the recipient, so mail is scoped to
		// the caller and a scope would add nothing.
		interceptor.Method(srvName + "ListMail"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "MarkRead"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "Claim"):    interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ClaimAll"): interceptor.ScopeAuthenticated,
	}
}
