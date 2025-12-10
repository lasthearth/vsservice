package service

import (
	"github.com/lasthearth/vsservice/internal/server/interceptor"
)

// Scope returns the authorization scopes required for the kit service methods
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/kit.v1.KitService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "GetAvailableKits"):     interceptor.Scope("kit:list"),
		interceptor.Method(srvName + "AssignKitToUser"):      interceptor.Scope("kit:assign"),
		interceptor.Method(srvName + "GetAssignmentStatus"):  interceptor.Scope("kit:read"),
		interceptor.Method(srvName + "ListUserAssignments"):  interceptor.Scope("kit:read"),
	}
}