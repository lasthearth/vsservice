package model

import (
	"fmt"
	"time"
)

// NotifierBlockDomain is the game mod domain (modid) the notifier block ships in.
const NotifierBlockDomain = "lhgui"

// NotifierBlockKind is the mail attachment type of the notifier: it is a block,
// not an item (the two namespaces can hold the same code).
const NotifierBlockKind = "block"

// NotifierBlockCode returns the game code of the notifier block tier that
// matches the settlement type, e.g. "lhgui:notifier-camp". The block is the
// "notifier" blocktype with a "tier" variant group (assets/lhgui/blocktypes/
// notifier.json), so the code is "<domain>:notifier-<tier>".
//
// ok is false for types outside the notifier automation. Today every
// SettlementType has a tier; hamlet (khutor) and the imperial tiers
// (abandoned / normal / mine) have no SettlementType, so they never get a block
// from the site and stay manual.
func (t SettlementType) NotifierBlockCode() (code string, ok bool) {
	var tier string
	switch t {
	case SettlementTypeCamp:
		tier = "camp"
	case SettlementTypeVillage:
		tier = "village"
	case SettlementTypeTownship:
		// The block variant is "town", not "township".
		tier = "town"
	case SettlementTypeCity:
		tier = "city"
	case SettlementTypeProvince:
		tier = "province"
	default:
		return "", false
	}
	return NotifierBlockDomain + ":notifier-" + tier, true
}

// Title is the Russian display name of the settlement type, for player-facing
// texts.
func (t SettlementType) Title() string {
	switch t {
	case SettlementTypeCamp:
		return "Лагерь"
	case SettlementTypeVillage:
		return "Деревня"
	case SettlementTypeTownship:
		return "Посёлок"
	case SettlementTypeCity:
		return "Город"
	case SettlementTypeProvince:
		return "Региональная провинция"
	default:
		return string(t)
	}
}

// NotifierDeliveryKey is the mail idempotency key of the notifier handed out
// when a settlement is created.
func NotifierDeliveryKey(settlementId string) string {
	return "settlement-notifier:" + settlementId
}

// NotifierReissueKey is the mail idempotency key of the n-th reissue.
func NotifierReissueKey(settlementId string, n int) string {
	return fmt.Sprintf("settlement-notifier:%s:%d", settlementId, n)
}

// Vector3 is a block position in the game world.
type Vector3 struct {
	X int
	Y int
	Z int
}

// NotifierPlacement is where a settlement's notifier block stands. It is
// written by the game server when the block is placed or broken; the site only
// reads it, so the model has no mutators.
type NotifierPlacement struct {
	SettlementId string
	Position     Vector3
	PlacedBy     string
	PlacedAt     time.Time
}

// Coordinates is the placement on the site's 2D map: world X and Z. World Y is
// the height and has no place on the map.
func (p NotifierPlacement) Coordinates() Vector2 {
	return Vector2{X: p.Position.X, Y: p.Position.Z}
}

// ResolveCoordinates returns the coordinates an upgrade request must carry.
// While a notifier stands, its position is authoritative and whatever the
// client submitted is dropped; with no notifier the submitted value is kept.
// locked reports whether the notifier overrode the input.
func ResolveCoordinates(submitted Vector2, placement *NotifierPlacement) (resolved Vector2, locked bool) {
	if placement == nil {
		return submitted, false
	}
	return placement.Coordinates(), true
}

// UpgradeNoticeKey is the mail idempotency key of the notice one owner gets when
// an upgrade request is approved. Mail keys are unique per mail, so it names the
// recipient too. requestedAt is when the request was submitted, so a second
// upgrade to a tier the settlement already held (after a downgrade on the site)
// still sends its notice.
func UpgradeNoticeKey(settlementId string, to SettlementType, requestedAt time.Time, userId string) string {
	return fmt.Sprintf("settlement-upgrade:%s:%s:%d:%s", settlementId, to, requestedAt.UnixMilli(), userId)
}
