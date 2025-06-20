package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/news.v1.NewsService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "CreateNews"): interceptor.Scope("news:create"),
	}
}
