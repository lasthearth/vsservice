package model

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrUnknownItem: an id is not in the catalog.
	ErrUnknownItem = errors.New("unknown appearance item")
	// ErrLocked: the player has not unlocked a chosen item.
	ErrLocked = errors.New("appearance item is locked")
	// ErrNotForSale: the item is earned, not bought.
	ErrNotForSale = errors.New("appearance item is not for sale")
	// ErrAlreadyOwned: the player has already bought the item.
	ErrAlreadyOwned = errors.New("appearance item is already owned")
)

// Choice is what the player picked.
type Choice struct {
	BannerId     string
	BannerEffect string
	FrameId      string
	FrameEffect  string
	TitleKey     string
}

// Appearance is one player's saved look.
type Appearance struct {
	UserId       string
	BannerId     string
	BannerEffect string
	FrameId      string
	FrameEffect  string
	TitleKey     string
	UpdatedAt    time.Time
}

// NewAppearance checks the choice against the catalog and the player's
// standing and returns the look to save.
func NewAppearance(userID string, choice Choice, standing Standing, now time.Time) (*Appearance, error) {
	a := &Appearance{UserId: userID}
	if err := a.Choose(choice, standing, now); err != nil {
		return nil, err
	}
	return a, nil
}

// Choose replaces the look with the choice, if every item exists and is
// unlocked for the standing.
func (a *Appearance) Choose(choice Choice, standing Standing, now time.Time) error {
	items := []struct {
		kind    string
		id      string
		catalog map[string]Requirement
	}{
		{"banner", choice.BannerId, Banners},
		{"banner animation", choice.BannerEffect, BannerEffects},
		{"frame", choice.FrameId, Frames},
		{"frame animation", choice.FrameEffect, FrameEffects},
	}
	for _, item := range items {
		// «Без баннера» — не элемент каталога, а отсутствие баннера:
		// условий не спрашивает, не продаётся.
		if item.kind == "banner" && item.id == BannerNone {
			continue
		}
		req, ok := item.catalog[item.id]
		if !ok {
			return fmt.Errorf("%w: %s %q", ErrUnknownItem, item.kind, item.id)
		}
		if !req.MetBy(standing, item.id) {
			return fmt.Errorf("%w: %s %q needs %s", ErrLocked, item.kind, item.id, req)
		}
	}
	if _, ok := Titles[choice.TitleKey]; !ok {
		return fmt.Errorf("%w: title %q", ErrUnknownItem, choice.TitleKey)
	}

	a.BannerId = choice.BannerId
	a.BannerEffect = choice.BannerEffect
	a.FrameId = choice.FrameId
	a.FrameEffect = choice.FrameEffect
	a.TitleKey = choice.TitleKey
	a.UpdatedAt = now
	return nil
}
