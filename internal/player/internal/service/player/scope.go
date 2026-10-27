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

		// SearchUsers passes the query into a Mongo $regex. The query is escaped
		// now, so a caller can no longer build a pattern and cannot make the regex
		// engine backtrack; it is still unanchored and has no minimum length, so it
		// remains a roster dump. Left authenticated-only.
		interceptor.Method(srvName + "SearchUsers"): interceptor.ScopeAuthenticated,

		// Self-service: the description belongs to the JWT subject, and the
		// service writes nothing else.
		interceptor.Method(srvName + "UpdateMyAbout"): interceptor.ScopeAuthenticated,

		// Hiding someone else's description is moderation. The scope has to be
		// granted to the moderator role in Logto; until it is, only a token that
		// already carries players:moderate can call this.
		interceptor.Method(srvName + "HideAbout"): interceptor.Scope("players:moderate"),

		// Renaming someone else is a narrower right than moderation: it is
		// granted to the support role, not to every moderator. Until the scope is
		// granted in Logto, only a token that already carries it can call this.
		interceptor.Method(srvName + "AdminChangeNickname"): interceptor.Scope("players:nickname:write"),

		// Lifting a player's cooldown lets them rename themselves, so it is
		// separate from renaming for them: support may grant a change without
		// choosing the name. Same Logto grant requirement as above.
		interceptor.Method(srvName + "AdminGrantNicknameChange"): interceptor.Scope("players:nickname:grant"),

		// ListPlayers is public (matcher.go): the site's /players page is read
		// without an account. It is not a repeat of the leaderboard - it adds
		// is_online, the game's live presence flag, the description the player
		// wrote, and a match on their former nickname. All three are what the page
		// is for (tasks/005). The takedown path is HideAbout, which answers
		// PERMISSION_DENIED until players:moderate is granted in Logto.
	}
}
