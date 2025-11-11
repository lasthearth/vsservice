package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/settlement.v1.SettlementTagService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "DeleteTag"): interceptor.Scope("tags:delete"),
		interceptor.Method(srvName + "CreateTag"): interceptor.Scope("tags:create"),
	}
}
