package model

import "time"

type VerificationStatus string

const (
	VerificationStatusPending  VerificationStatus = "pending"
	VerificationStatusApproved VerificationStatus = "approved"
	VerificationStatusVerified VerificationStatus = "verified"
	VerificationStatusRejected VerificationStatus = "rejected"
)

// Verification represents a verification record for a user.
type Verification struct {
	ID string
	// User id from sso
	UserID          string
	UserName        string
	UserGameName    string
	Answers         []Answer
	Contacts        string
	Status          VerificationStatus
	RejectionReason string
	UpdatedAt       time.Time
	CreatedAt       time.Time
}

type Answer struct {
	ID        string
	Question  string
	Answer    string
	CreatedAt time.Time
}
