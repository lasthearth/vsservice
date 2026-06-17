package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srv := "/imperialpoint.v1.ImperialPointService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srv + "CreatePoint"):    interceptor.Scope("imperialpoint:write"),
		interceptor.Method(srv + "UpdatePoint"):    interceptor.Scope("imperialpoint:write"),
		interceptor.Method(srv + "SetControl"):     interceptor.Scope("imperialpoint:write"),
		interceptor.Method(srv + "ReleaseControl"): interceptor.Scope("imperialpoint:write"),
	}
}
