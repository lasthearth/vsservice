package model

// SettlementKind tells the two kinds of group apart. A guild is stored as a settlement document
// and shares every mechanic with one — profile, roles, invitations, join requests, invite links,
// moderation, notifier block. What the kind decides is the membership rules and which of the two
// per-kind unique indexes a document answers to.
//
// The kind is derived from Type and never set on its own, so the two can never disagree. The
// stored copy in the database exists because a unique partial index can only filter on the
// equality of a stored field, and that index is what enforces one settlement plus one guild per
// player.
type SettlementKind string

const (
	// KindSettlement covers every settlement tier: camp, village, township, city, province.
	KindSettlement SettlementKind = "settlement"
	// KindGuild covers the guild tier.
	KindGuild SettlementKind = "guild"
	// KindAny selects both kinds in a listing. It is never stored and never answers a membership
	// rule, so MayJoin refuses it.
	KindAny SettlementKind = ""
)

// Kind returns the kind a settlement type belongs to. The guild tier is the only guild; every
// other tier, known or added later, is a settlement.
func (t SettlementType) Kind() SettlementKind {
	if t == SettlementTypeGuild {
		return KindGuild
	}
	return KindSettlement
}

// IsGuild reports whether the type is the guild tier.
func (t SettlementType) IsGuild() bool {
	return t.Kind() == KindGuild
}

// Kind returns the kind of this group.
func (s *Settlement) Kind() SettlementKind {
	return s.Type.Kind()
}

// IsGuild reports whether this group is a guild.
func (s *Settlement) IsGuild() bool {
	return s.Type.IsGuild()
}

// Kind returns the kind a verification request asks for. It is the kind the approved group will
// have, since approval copies the request's type onto the settlement.
func (s *SettlementVerification) Kind() SettlementKind {
	return s.Type.Kind()
}
