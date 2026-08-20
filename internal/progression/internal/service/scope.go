package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope declares required JWT scopes for both gRPC services backed by *Service.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	prog := "/progression.v1.ProgressionService/"
	point := "/imperialpoint.v1.ImperialPointService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(prog + "CreateTree"):             interceptor.Scope("progression:write"),
		interceptor.Method(prog + "UpdateTree"):             interceptor.Scope("progression:write"),
		interceptor.Method(prog + "CreatePreset"):           interceptor.Scope("progression:write"),
		interceptor.Method(prog + "UpdatePreset"):           interceptor.Scope("progression:write"),
		interceptor.Method(prog + "PurchaseSettlementNode"): interceptor.Scope(""),
		interceptor.Method(prog + "PurchasePointNode"):      interceptor.Scope(""),

		interceptor.Method(point + "CreatePoint"):    interceptor.Scope("imperialpoint:write"),
		interceptor.Method(point + "UpdatePoint"):    interceptor.Scope("imperialpoint:write"),
		interceptor.Method(point + "SetControl"):     interceptor.Scope("imperialpoint:write"),
		interceptor.Method(point + "ReleaseControl"): interceptor.Scope("imperialpoint:write"),
	}
}
