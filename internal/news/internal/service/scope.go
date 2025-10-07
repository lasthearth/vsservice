package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope implements interceptor.Scoper.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/news.v1.NewsService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "CreateNews"): interceptor.Scope("news:create"),
		interceptor.Method(srvName + "DeleteNews"): interceptor.Scope("news:delete"),
	}
}
