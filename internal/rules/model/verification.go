package model

import "time"

// Verification represents a verification record for a user.
type Verification struct {
	ID string
	// User id from sso
	UserID    string
	Answers   []Answer
	UpdatedAt time.Time
	CreatedAt time.Time
}

type Answer struct {
	ID        string
	Question  string
	Answer    string
	CreatedAt time.Time
}
