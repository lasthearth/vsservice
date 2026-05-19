package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/hungergames.v1.HungerGamesService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "RecordMatch"):  interceptor.Scope("hungergames:match:record"),
		interceptor.Method(srvName + "ResetSeason"):  interceptor.Scope("hungergames:season:reset"),
		interceptor.Method(srvName + "CreateSeason"): interceptor.Scope("hungergames:season:create"),
	}
}
