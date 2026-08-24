package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope implements interceptor.Scoper.
//
// None of these carry a scope: UpdateAvatar and ChangeNickname reject a
// user_id that is not the JWT subject, and SearchUsers is a lookup over the
// public player roster. They are listed rather than omitted so a future method
// cannot become reachable by simply not appearing here — see
// interceptor.Auth.uncoveredMethods.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/user.v1.UserService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "UpdateAvatar"):   interceptor.ScopeAuthenticated,
		interceptor.Method(srvName + "ChangeNickname"): interceptor.ScopeAuthenticated,

		// SearchUsers passes the query straight into a Mongo $regex with no
		// escaping, anchoring or minimum length, so it is both a roster dump and
		// a ReDoS vector. Left authenticated-only to keep this change
		// behaviour-preserving; tracked separately.
		interceptor.Method(srvName + "SearchUsers"): interceptor.ScopeAuthenticated,
	}
}
