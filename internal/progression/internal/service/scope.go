package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

// Scope declares required JWT scopes for both gRPC services backed by *Service.
func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	prog := "/progression.v1.ProgressionService/"
	point := "/imperialpoint.v1.ImperialPointService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(prog + "CreateTree"):             interceptor.Scope("progression:write"),
		interceptor.Method(prog + "UpdateTree"):             interceptor.Scope("progression:write"),
		interceptor.Method(prog + "CreatePreset"):           interceptor.Scope("progression:write"),
		interceptor.Method(prog + "UpdatePreset"):           interceptor.Scope("progression:write"),
		interceptor.Method(prog + "PurchaseSettlementNode"): interceptor.ScopeAuthenticated,
		interceptor.Method(prog + "PurchasePointNode"):      interceptor.ScopeAuthenticated,

		// Read-only views of the shared talent trees and presets.
		interceptor.Method(prog + "GetTree"):     interceptor.ScopeAuthenticated,
		interceptor.Method(prog + "ListTrees"):   interceptor.ScopeAuthenticated,
		interceptor.Method(prog + "GetPreset"):   interceptor.ScopeAuthenticated,
		interceptor.Method(prog + "ListPresets"): interceptor.ScopeAuthenticated,

		// Progress lookups take a settlement/point id and tree id from the
		// request. GetOrCreateProgress inserts an empty document when none
		// matches, and only the id FORMAT is validated, not existence — so any
		// authenticated caller can create unbounded progress documents under
		// arbitrary ids. This is a storage-growth issue, not a disclosure. Left
		// authenticated-only here; the read-that-writes is tracked separately.
		interceptor.Method(prog + "GetSettlementProgress"): interceptor.ScopeAuthenticated,
		interceptor.Method(prog + "GetPointProgress"):      interceptor.ScopeAuthenticated,

		interceptor.Method(point + "CreatePoint"):    interceptor.Scope("imperialpoint:write"),
		interceptor.Method(point + "UpdatePoint"):    interceptor.Scope("imperialpoint:write"),
		interceptor.Method(point + "SetControl"):     interceptor.Scope("imperialpoint:write"),
		interceptor.Method(point + "ReleaseControl"): interceptor.Scope("imperialpoint:write"),

		// Read-only views of the imperial point map.
		interceptor.Method(point + "GetPoint"):   interceptor.ScopeAuthenticated,
		interceptor.Method(point + "ListPoints"): interceptor.ScopeAuthenticated,
	}
}
