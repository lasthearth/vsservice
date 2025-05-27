package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/settlement.v1.SettlementService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "Approve"):      interceptor.Scope("settlements:manage"),
		interceptor.Method(srvName + "ListPending"):  interceptor.Scope("settlements:manage"),
		interceptor.Method(srvName + "Reject"):       interceptor.Scope("settlements:manage"),
		interceptor.Method(srvName + "RemoveMember"): interceptor.Scope("settlements:manage"),
	}
}
