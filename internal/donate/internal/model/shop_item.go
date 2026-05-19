package model

import (
	"errors"
	"time"
)

// ShopItem is an item available for purchase in the donate shop.
type ShopItem struct {
	Id          string
	Code        string
	Name        string
	Description string
	ImageURL    string
	Price       int64
	IsAvailable bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewShopItem(code, name, description, imageURL string, price int64) *ShopItem {
	return &ShopItem{
		Code:        code,
		Name:        name,
		Description: description,
		ImageURL:    imageURL,
		Price:       price,
		IsAvailable: true,
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
	return nil
}

func (s *ShopItem) Update(code, name, description, imageURL string, price int64, isAvailable bool) {
	s.Code = code
	s.Name = name
	s.Description = description
	s.ImageURL = imageURL
	s.Price = price
	s.IsAvailable = isAvailable
}
