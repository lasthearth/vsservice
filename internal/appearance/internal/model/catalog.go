package model

import "fmt"

// RequirementKind is what an item asks of the player.
type RequirementKind int

const (
	RequireFree RequirementKind = iota
	RequireHours
	RequireKills
	RequireSettlement
	RequireLeader
	RequireTop
	RequireDeaths
	RequireSurvivor
	RequireHungerGamesWins
	RequireReferrals
	RequireEvents
	RequireDays
	// RequirePurchase: bought with shards; Amount is the price.
	RequirePurchase
)

// Requirement unlocks one item. Amount is the number the kind asks for:
// hours, kills, deaths, wins, invited players, events, days or the price.
type Requirement struct {
	Kind   RequirementKind
	Amount int
}

// MetBy reports whether the standing satisfies the requirement for the item.
func (r Requirement) MetBy(s Standing, itemID string) bool {
	switch r.Kind {
	case RequireFree:
		return true
	case RequireHours:
		return s.Hours >= float64(r.Amount)
	case RequireKills:
		return s.Kills >= r.Amount
	case RequireSettlement:
		return s.SettlementRole != SettlementNone
	case RequireLeader:
		return s.SettlementRole == SettlementLeader
	case RequireTop:
		return s.inTop()
	case RequireDeaths:
		return s.Deaths >= r.Amount
	case RequireSurvivor:
		return s.survives(r.Amount)
	case RequireHungerGamesWins:
		return s.HungerGamesWins >= r.Amount
	case RequireReferrals:
		return s.Referrals >= r.Amount
	case RequireEvents:
		return s.Events >= r.Amount
	case RequireDays:
		return s.Days >= r.Amount
	case RequirePurchase:
		return s.Owns(itemID)
	default:
		return false
	}
}

// String describes the requirement for error messages.
func (r Requirement) String() string {
	switch r.Kind {
	case RequireHours:
		return fmt.Sprintf("%d hours in game", r.Amount)
	case RequireKills:
		return fmt.Sprintf("%d player kills", r.Amount)
	case RequireSettlement:
		return "membership in a settlement"
	case RequireLeader:
		return "leading a settlement"
	case RequireTop:
		return fmt.Sprintf("a top-%d place by hours or kills", TopRank)
	case RequireDeaths:
		return fmt.Sprintf("%d deaths", r.Amount)
	case RequireSurvivor:
		return fmt.Sprintf("%d hours with at most one death per ten hours", r.Amount)
	case RequireHungerGamesWins:
		return fmt.Sprintf("%d Hunger Games wins", r.Amount)
	case RequireReferrals:
		return fmt.Sprintf("%d invited players", r.Amount)
	case RequireEvents:
		return fmt.Sprintf("signing up for %d events", r.Amount)
	case RequireDays:
		return fmt.Sprintf("%d days on the server", r.Amount)
	case RequirePurchase:
		return fmt.Sprintf("buying it for %d shards", r.Amount)
	default:
		return "nothing"
	}
}

// BannerPrice returns the shard price of a banner sold in the shop.
func BannerPrice(bannerID string) (int64, error) {
	req, ok := Banners[bannerID]
	if !ok {
		return 0, fmt.Errorf("%w: banner %q", ErrUnknownItem, bannerID)
	}
	if req.Kind != RequirePurchase {
		return 0, fmt.Errorf("%w: banner %q", ErrNotForSale, bannerID)
	}
	return int64(req.Amount), nil
}

// The catalogs mirror the site (entities/player-style/lib/player-style.constant.ts):
// the same ids and the same unlocks.

// BannerShardPrice is what each banner sold for shards costs.
const BannerShardPrice = 3000

// BannerNone is the player's choice to show no banner at all. It mirrors the
// site's BANNER_NONE (entities/player-style/lib/player-style.constant.ts):
// not a catalog item, so it is never for sale and never counts as the one
// free banner — just the absence of a banner.
const BannerNone = "none"

// Banners by id. Only one is open from the start; most are earned in the game
// or on the site, three are bought with shards.
var Banners = map[string]Requirement{
	"procession":       {Kind: RequireFree},
	"jesters":          {Kind: RequireHours, Amount: 5},
	"wrestlers":        {Kind: RequireKills, Amount: 3},
	"merry-company":    {Kind: RequireEvents, Amount: 3},
	"unicorn-tapestry": {Kind: RequireDays, Amount: 30},
	"shot-monkey":      {Kind: RequireDeaths, Amount: 10},
	"mummers":          {Kind: RequireReferrals, Amount: 1},
	"scholar-cat":      {Kind: RequireSettlement},
	"duel":             {Kind: RequireKills, Amount: 10},
	"monk-lion":        {Kind: RequireDays, Amount: 90},
	"odd-joust":        {Kind: RequireHungerGamesWins, Amount: 1},
	"bad-manners":      {Kind: RequireDeaths, Amount: 25},
	"bestiary":         {Kind: RequireSurvivor, Amount: 50},
	"war-horse":        {Kind: RequireLeader},
	"gallows":          {Kind: RequireKills, Amount: 25},
	"temptation":       {Kind: RequireHours, Amount: 300},
	"battle-courtrai":  {Kind: RequireKills, Amount: 50},
	"wonder-city":      {Kind: RequireTop},
	"azure-goat":       {Kind: RequirePurchase, Amount: BannerShardPrice},
	"three-dead":       {Kind: RequirePurchase, Amount: BannerShardPrice},
	"piper":            {Kind: RequirePurchase, Amount: BannerShardPrice},
}

// BannerEffects by id.
var BannerEffects = map[string]Requirement{
	"none":      {Kind: RequireFree},
	"dust":      {Kind: RequireFree},
	"clouds":    {Kind: RequireHours, Amount: 10},
	"leaves":    {Kind: RequireHours, Amount: 30},
	"rain":      {Kind: RequireHours, Amount: 60},
	"fog":       {Kind: RequireHours, Amount: 100},
	"snow":      {Kind: RequireKills, Amount: 10},
	"fireflies": {Kind: RequireSettlement},
	"embers":    {Kind: RequireHours, Amount: 250},
	"lightning": {Kind: RequireKills, Amount: 50},
	"starfall":  {Kind: RequireDays, Amount: 60},
	"aurora":    {Kind: RequireHours, Amount: 150},
}

// Frames by id.
var Frames = map[string]Requirement{
	"none":   {Kind: RequireFree},
	"wood":   {Kind: RequireFree},
	"copper": {Kind: RequireHours, Amount: 10},
	"bronze": {Kind: RequireHours, Amount: 100},
	"iron":   {Kind: RequireHours, Amount: 500},
	"blood":  {Kind: RequireKills, Amount: 50},
	"gold":   {Kind: RequireTop},
}

// FrameEffects by id.
var FrameEffects = map[string]Requirement{
	"none":   {Kind: RequireFree},
	"wind":   {Kind: RequireHours, Amount: 20},
	"fire":   {Kind: RequireHours, Amount: 50},
	"fog":    {Kind: RequireSettlement},
	"frost":  {Kind: RequireKills, Amount: 25},
	"storm":  {Kind: RequireTop},
	"comet":  {Kind: RequireKills, Amount: 30},
	"aurora": {Kind: RequireSurvivor, Amount: 50},
}

// Titles are the badge keys a player may show under the name; empty means no
// title. Owning the badge is checked by the site when it draws the title.
var Titles = map[string]struct{}{
	"":          {},
	"verified":  {},
	"newcomer":  {},
	"settler":   {},
	"veteran":   {},
	"oldTimer":  {},
	"founder":   {},
	"resident":  {},
	"warrior":   {},
	"slayer":    {},
	"survivor":  {},
	"topHours":  {},
	"topKills":  {},
	"gladiator": {},
}
