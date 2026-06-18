package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srv := "/progression.v1.ProgressionService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srv + "CreateTree"):             interceptor.Scope("progression:write"),
		interceptor.Method(srv + "UpdateTree"):             interceptor.Scope("progression:write"),
		interceptor.Method(srv + "CreatePreset"):           interceptor.Scope("progression:write"),
		interceptor.Method(srv + "UpdatePreset"):           interceptor.Scope("progression:write"),
		interceptor.Method(srv + "PurchaseSettlementNode"): interceptor.Scope(""),
		interceptor.Method(srv + "PurchasePointNode"):      interceptor.Scope(""),
	}
}
