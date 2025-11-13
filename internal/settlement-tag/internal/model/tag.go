package model

import (
	"fmt"
	"strings"
	"time"
)

// Tag represents a label that can be applied to settlements for categorization and filtering
//
// Can be created with the NewTag function.
type Tag struct {
	Id          string
	Name        string
	Color       Color
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	IsActive    bool
}

type Color struct {
	Red, Green, Blue float32
	Alpha            float32
}

// Validate ensures the tag meets the required constraints
func (t *Tag) Validate() error {
	name := strings.TrimSpace(t.Name)
	if name == "" {
		return fmt.Errorf("tag name cannot be empty")
	}

	if len(t.Name) < 1 || len(t.Name) > 50 {
		return fmt.Errorf("tag name must be between 1 and 50 characters")
	}

	return nil
}

// NewTag creates a new tag with proper initialization
func NewTag(name, description string, color Color) (*Tag, error) {
	tag := &Tag{
		Name:        name,
		Color:       color,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		IsActive:    true,
	}

	if err := tag.Validate(); err != nil {
		return nil, err
	}

	return tag, nil
}
