package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope requires the admin kit:edit scope on both rpcs — this domain is
// admin-only kit metadata management.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/kitdef.v1.KitDefService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "ListKits"):  interceptor.Scope("kit:edit"),
		interceptor.Method(srvName + "RenameKit"): interceptor.Scope("kit:edit"),
	}
}
