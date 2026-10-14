package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope implements interceptor.Scoper.
//
// Managing the mod list reuses news:create rather than a scope of its own: the
// people who publish the site's news are the people who keep its mods, and a
// new scope would have to be granted in Logto before the tab worked at all.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/mod.v1.ModService/"
	manage := interceptor.Scope("news:create")

	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "ListModsAdmin"): manage,
		interceptor.Method(srvName + "LookupMod"):     manage,
		interceptor.Method(srvName + "AddMod"):        manage,
		interceptor.Method(srvName + "UpdateMod"):     manage,
		interceptor.Method(srvName + "DeleteMod"):     manage,
		interceptor.Method(srvName + "ReorderMods"):   manage,
		interceptor.Method(srvName + "RefreshMods"):   manage,

		// ListMods is public (matcher.go): it is the page every player reads.
	}
}
