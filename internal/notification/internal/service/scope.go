package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope implements interceptor.Scoper.
//
// Both methods are self-service and carry no scope: ListNotifications filters on
// the JWT subject, and MarkAsRead is bound to the caller in the same way. They
// are listed rather than omitted so a future method cannot become reachable by
// simply not appearing here — see interceptor.Auth.uncoveredMethods.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/notification.v1.NotificationService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "ListNotifications"): interceptor.ScopeAuthenticated,

		// MarkAsRead filters on _id plus the JWT subject at the database
		// layer, so another player's id is a silent no-op: nothing is written
		// and the answer does not tell the caller whether the id exists.
		interceptor.Method(srvName + "MarkAsRead"): interceptor.ScopeAuthenticated,
	}
}
