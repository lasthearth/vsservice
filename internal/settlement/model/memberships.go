package model

import "errors"

var (
	// ErrAlreadyInSettlement reports that the player already belongs to a settlement.
	ErrAlreadyInSettlement = errors.New("player already belongs to a settlement")
	// ErrAlreadyInGuild reports that the player already belongs to a guild.
	ErrAlreadyInGuild = errors.New("player already belongs to a guild")
	// ErrGuildOwnerInSettlement reports the one rule that crosses the two kinds: the owner of a
	// guild belongs to no settlement. Guild members do live in settlements, so this binds only
	// founding a guild, owning one and taking its ownership.
	ErrGuildOwnerInSettlement = errors.New("the owner of a guild cannot belong to a settlement")
	// ErrUnknownKind reports a kind that is neither settlement nor guild. The kind is derived from
	// a settlement type, so only a caller that invents one can reach it.
	ErrUnknownKind = errors.New("unknown group kind")
)

// Memberships is what one player holds across all groups. It is read from the stored documents —
// at most one settlement and at most one guild, because each kind has its own unique index — and
// it carries no identity: the rules below are the only place that decides what a player may join.
type Memberships struct {
	// InSettlement is true while the player belongs to a settlement, as a member or as an owner.
	InSettlement bool
	// InGuild is true while the player belongs to a guild, as a member or as an owner.
	InGuild bool
	// OwnsGuild is true while the player holds the owner role of a guild.
	OwnsGuild bool
}

// MayJoin reports whether the player may become a member of a group of this kind.
//
// A player holds at most one group per kind. On top of that, a guild owner stays out of
// settlements: joining one would break the rule that the owner of a guild belongs to no
// settlement.
func (m Memberships) MayJoin(kind SettlementKind) error {
	switch kind {
	case KindGuild:
		if m.InGuild {
			return ErrAlreadyInGuild
		}
	case KindSettlement:
		if m.InSettlement {
			return ErrAlreadyInSettlement
		}
		if m.OwnsGuild {
			return ErrGuildOwnerInSettlement
		}
	default:
		return ErrUnknownKind
	}
	return nil
}

// MayOwnGuild reports whether the player may found a guild or take the owner role of one.
//
// Belonging to a guild already is not a reason to refuse: transfer and grant both name a member
// of that guild. Only a settlement membership is, and the player must leave it first.
func (m Memberships) MayOwnGuild() error {
	if m.InSettlement {
		return ErrGuildOwnerInSettlement
	}
	return nil
}
