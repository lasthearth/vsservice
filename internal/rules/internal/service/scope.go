package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/rules.v1.RuleService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "CreateQuestion"): interceptor.Scope("question:create"),
	}
}
