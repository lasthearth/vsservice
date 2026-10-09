package model_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/appearance/internal/model"
)

var now = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

// pick builds a choice: banner, banner animation, frame, frame animation, title.
func pick(banner, bannerEffect, frame, frameEffect, title string) model.Choice {
	return model.Choice{
		BannerId:     banner,
		BannerEffect: bannerEffect,
		FrameId:      frame,
		FrameEffect:  frameEffect,
		TitleKey:     title,
	}
}

func basic() model.Choice {
	return pick("procession", "none", "wood", "none", "")
}

// «Без баннера» — не элемент каталога, а его отсутствие: сохраняется при
// любом standing и не влияет на «ровно один бесплатный баннер».
func TestNewAppearanceAcceptsNoBanner(t *testing.T) {
	a, err := model.NewAppearance("u1", pick("none", "none", "wood", "none", ""), model.Standing{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.BannerId != "none" {
		t.Fatalf("BannerId = %q, want none", a.BannerId)
	}
}

// A newcomer gets exactly one banner: the rest are earned.
func TestOnlyOneBannerIsFree(t *testing.T) {
	var free []string
	for id, req := range model.Banners {
		if req.MetBy(model.Standing{}, id) {
			free = append(free, id)
		}
	}
	if len(free) != 1 || free[0] != "procession" {
		t.Fatalf("free banners = %v, want [procession]", free)
	}
}

func TestNewAppearanceSavesUnlockedChoice(t *testing.T) {
	a, err := model.NewAppearance("u1", pick("procession", "none", "wood", "none", "verified"), model.Standing{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.UserId != "u1" || a.BannerId != "procession" || a.FrameId != "wood" || a.TitleKey != "verified" {
		t.Fatalf("unexpected appearance %+v", a)
	}
	if !a.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", a.UpdatedAt, now)
	}
}

func TestNewAppearanceRejectsUnknownItems(t *testing.T) {
	cases := map[string]model.Choice{
		"banner":           pick("abbey-ruins", "none", "wood", "none", ""),
		"banner animation": pick("procession", "volcano", "wood", "none", ""),
		"frame":            pick("procession", "none", "diamond", "none", ""),
		"frame animation":  pick("procession", "none", "wood", "rainbow", ""),
		"title":            pick("procession", "none", "wood", "none", "emperor"),
	}
	for name, choice := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := model.NewAppearance("u1", choice, model.Standing{}, now); !errors.Is(err, model.ErrUnknownItem) {
				t.Fatalf("err = %v, want ErrUnknownItem", err)
			}
		})
	}
}

func TestUnlocks(t *testing.T) {
	cases := []struct {
		name     string
		choice   model.Choice
		locked   model.Standing
		unlocked model.Standing
	}{
		{
			name:     "hours",
			choice:   pick("temptation", "none", "wood", "none", ""),
			locked:   model.Standing{Hours: 299.9},
			unlocked: model.Standing{Hours: 300},
		},
		{
			name:     "kills",
			choice:   pick("procession", "none", "blood", "none", ""),
			locked:   model.Standing{Kills: 49},
			unlocked: model.Standing{Kills: 50},
		},
		{
			name:     "settlement",
			choice:   pick("procession", "fireflies", "wood", "none", ""),
			locked:   model.Standing{},
			unlocked: model.Standing{SettlementRole: model.SettlementResident},
		},
		{
			name:     "leader",
			choice:   pick("war-horse", "none", "wood", "none", ""),
			locked:   model.Standing{SettlementRole: model.SettlementResident},
			unlocked: model.Standing{SettlementRole: model.SettlementLeader},
		},
		{
			name:     "top by hours",
			choice:   pick("procession", "none", "wood", "storm", ""),
			locked:   model.Standing{HoursRank: 11, KillsRank: 40},
			unlocked: model.Standing{HoursRank: 10, KillsRank: 40},
		},
		{
			name:     "deaths",
			choice:   pick("shot-monkey", "none", "wood", "none", ""),
			locked:   model.Standing{Deaths: 9},
			unlocked: model.Standing{Deaths: 10},
		},
		{
			name:     "survivor needs the hours",
			choice:   pick("bestiary", "none", "wood", "none", ""),
			locked:   model.Standing{Hours: 49, Deaths: 0},
			unlocked: model.Standing{Hours: 50, Deaths: 5},
		},
		{
			name:     "survivor dies rarely",
			choice:   pick("bestiary", "none", "wood", "none", ""),
			locked:   model.Standing{Hours: 80, Deaths: 9},
			unlocked: model.Standing{Hours: 80, Deaths: 8},
		},
		{
			name:     "hunger games wins",
			choice:   pick("odd-joust", "none", "wood", "none", ""),
			locked:   model.Standing{},
			unlocked: model.Standing{HungerGamesWins: 1},
		},
		{
			name:     "referrals",
			choice:   pick("mummers", "none", "wood", "none", ""),
			locked:   model.Standing{},
			unlocked: model.Standing{Referrals: 1},
		},
		{
			name:     "events",
			choice:   pick("merry-company", "none", "wood", "none", ""),
			locked:   model.Standing{Events: 2},
			unlocked: model.Standing{Events: 3},
		},
		{
			name:     "days",
			choice:   pick("monk-lion", "none", "wood", "none", ""),
			locked:   model.Standing{Days: 89},
			unlocked: model.Standing{Days: 90},
		},
		{
			name:     "purchase",
			choice:   pick("piper", "none", "wood", "none", ""),
			locked:   model.Standing{Hours: 9999, Purchased: map[string]bool{"three-dead": true}},
			unlocked: model.Standing{Purchased: map[string]bool{"piper": true}},
		},
		{
			name:     "top by kills",
			choice:   pick("procession", "none", "gold", "none", ""),
			locked:   model.Standing{},
			unlocked: model.Standing{HoursRank: 200, KillsRank: 1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			choice := tc.choice
			if _, err := model.NewAppearance("u1", choice, tc.locked, now); !errors.Is(err, model.ErrLocked) {
				t.Fatalf("locked: err = %v, want ErrLocked", err)
			}
			if _, err := model.NewAppearance("u1", choice, tc.unlocked, now); err != nil {
				t.Fatalf("unlocked: %v", err)
			}
		})
	}
}

// A rejected choice leaves the saved look as it was.
func TestChooseKeepsLookOnError(t *testing.T) {
	a, err := model.NewAppearance("u1", basic(), model.Standing{}, now)
	if err != nil {
		t.Fatal(err)
	}
	locked := pick("wonder-city", "none", "wood", "none", "")
	if err := a.Choose(locked, model.Standing{}, now.Add(time.Hour)); !errors.Is(err, model.ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
	if a.BannerId != "procession" || !a.UpdatedAt.Equal(now) {
		t.Fatalf("look changed after a rejected choice: %+v", a)
	}
}

func TestBannerPrice(t *testing.T) {
	price, err := model.BannerPrice("piper")
	if err != nil || price != model.BannerShardPrice {
		t.Fatalf("piper: price %d, err %v", price, err)
	}
	if _, err := model.BannerPrice("duel"); !errors.Is(err, model.ErrNotForSale) {
		t.Fatalf("duel: err = %v, want ErrNotForSale", err)
	}
	if _, err := model.BannerPrice("abbey-ruins"); !errors.Is(err, model.ErrUnknownItem) {
		t.Fatalf("abbey-ruins: err = %v, want ErrUnknownItem", err)
	}
}

// Exactly three banners are sold, all at the same price.
func TestThreeBannersForShards(t *testing.T) {
	sold := 0
	for id, req := range model.Banners {
		if req.Kind == model.RequirePurchase {
			sold++
			if req.Amount != model.BannerShardPrice {
				t.Fatalf("%s costs %d", id, req.Amount)
			}
		}
	}
	if sold != 3 {
		t.Fatalf("%d banners for shards, want 3", sold)
	}
}
