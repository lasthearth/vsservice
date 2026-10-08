package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope reuses the news scopes: whoever publishes news also runs the calendar,
// and no new Logto permission has to be configured. Signing up and "my events"
// need a signed-in player. GetEvent, ListEvents and ListAttendees are public
// (see interceptor publicMethods).
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/event.v1.EventService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "CreateEvent"):   interceptor.Scope("news:create"),
		interceptor.Method(srvName + "UpdateEvent"):   interceptor.Scope("news:create"),
		interceptor.Method(srvName + "DeleteEvent"):   interceptor.Scope("news:delete"),
		interceptor.Method(srvName + "SetAttendance"): interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ListMyEvents"):  interceptor.ScopeAuthenticated,
	}
}
