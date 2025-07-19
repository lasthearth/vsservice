package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/verification.v1.VerificationService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "Approve"): interceptor.Scope("user:verify"),
		interceptor.Method(srvName + "List"):    interceptor.Scope("user:verify"),
		interceptor.Method(srvName + "Reject"):  interceptor.Scope("user:verify"),
	}
}
