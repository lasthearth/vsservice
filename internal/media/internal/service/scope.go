package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope implements interceptor.Scoper. The method only requires authentication;
// authorization is per-purpose inside CreateUploadUrls (see checkScope).
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	return map[interceptor.Method]interceptor.Scope{}
}
