package model

import (
	"errors"
	"fmt"
	"time"
)

type ItemType string

const (
	ItemTypeItem ItemType = "item"
	ItemTypeKit  ItemType = "kit"
)

// KitEntry describes a single item inside a kit.
type KitEntry struct {
	Name        string
	Description string
	ImageURL    string
	Quantity    int32
}

// ShopItemUpdate carries all updatable fields for Apply.
type ShopItemUpdate struct {
	Code, Name, Description, ImageURL string
	Price                             int64
	IsAvailable                       bool
	Type                              ItemType
	Entries                           []KitEntry
	HasDiscount                       bool
	DiscountPercent                   int32
}

// ShopItem is an item available for purchase in the donate shop.
type ShopItem struct {
	Id              string
	Code            string
	Name            string
	Description     string
	ImageURL        string
	Price           int64
	IsAvailable     bool
	Type            ItemType
	Entries         []KitEntry
	HasDiscount     bool
	DiscountPercent int32
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewShopItem(code, name, description, imageURL string, price int64) *ShopItem {
	return &ShopItem{
		Code:        code,
		Name:        name,
		Description: description,
		ImageURL:    imageURL,
		Price:       price,
		IsAvailable: true,
		Type:        ItemTypeItem,
	}
}

func NewKitShopItem(code, name, description, imageURL string, price int64, entries []KitEntry) *ShopItem {
	return &ShopItem{
		Code:        code,
		Name:        name,
		Description: description,
		ImageURL:    imageURL,
		Price:       price,
		IsAvailable: true,
		Type:        ItemTypeKit,
		Entries:     entries,
	}
}

func (s *ShopItem) Validate() error {
	if s.Code == "" {
		return errors.New("code cannot be empty")
	}
	if s.Name == "" {
		return errors.New("name cannot be empty")
	}
	if s.Price <= 0 {
		return errors.New("price must be positive")
	}
	if s.HasDiscount && (s.DiscountPercent < 0 || s.DiscountPercent > 100) {
		return errors.New("discount_percent must be between 0 and 100")
	}
	if s.Type == ItemTypeKit {
		if len(s.Entries) == 0 {
			return errors.New("kit must have at least one entry")
		}
		for i, e := range s.Entries {
			if e.Name == "" {
				return errors.New("kit entry name cannot be empty")
			}
			if e.Quantity <= 0 {
				return fmt.Errorf("kit entry %d quantity must be positive", i)
			}
		}
	}
	return nil
}

// Apply updates all fields from the provided ShopItemUpdate.
func (s *ShopItem) Apply(u ShopItemUpdate) {
	s.Code = u.Code
	s.Name = u.Name
	s.Description = u.Description
	s.ImageURL = u.ImageURL
	s.Price = u.Price
	s.IsAvailable = u.IsAvailable
	s.Type = u.Type
	s.Entries = u.Entries
	s.HasDiscount = u.HasDiscount
	s.DiscountPercent = u.DiscountPercent
}

// SetDiscount sets the discount percent (0..100) and marks HasDiscount=true.
func (s *ShopItem) SetDiscount(percent int32) error {
	if percent < 0 || percent > 100 {
		return errors.New("discount_percent must be between 0 and 100")
	}
	s.HasDiscount = true
	s.DiscountPercent = percent
	return nil
}

// ClearDiscount removes the discount.
func (s *ShopItem) ClearDiscount() {
	s.HasDiscount = false
	s.DiscountPercent = 0
}

// SetEntries validates and sets the kit entries.
func (s *ShopItem) SetEntries(e []KitEntry) error {
	for i, entry := range e {
		if entry.Name == "" {
			return errors.New("kit entry name cannot be empty")
		}
		if entry.Quantity <= 0 {
			return fmt.Errorf("kit entry %d quantity must be positive", i)
		}
	}
	s.Entries = e
	return nil
}

// EffectivePrice returns the price after applying the discount.
func (s *ShopItem) EffectivePrice() int64 {
	if !s.HasDiscount || s.DiscountPercent <= 0 {
		return s.Price
	}
	p := s.Price * int64(100-s.DiscountPercent) / 100
	if p < 1 {
		p = 1
	}
	return p
}
