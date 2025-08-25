package verification

import (
	"time"

	"github.com/go-faster/errors"
)

type VerificationStatus string

const (
	VerificationStatusPending  VerificationStatus = "pending"
	VerificationStatusApproved VerificationStatus = "approved"
	VerificationStatusVerified VerificationStatus = "verified"
	VerificationStatusRejected VerificationStatus = "rejected"
)

type Verification struct {
	Id string
	// User id from sso
	UserId           string
	Answers          []Answer
	Contacts         string
	Status           VerificationStatus
	RejectionReason  string
	VerificationCode string
	UpdatedAt        time.Time
	CreatedAt        time.Time
}

// CanSubmit checks if the verification can be submitted.
// Returns nil if the verification can be submitted.
func (v *Verification) CanSubmit() error {
	switch v.Status {
	case VerificationStatusRejected:
		return nil
	case VerificationStatusPending:
		return ErrVerificationPending
	case VerificationStatusApproved:
		return ErrAlreadyVerified
	case VerificationStatusVerified:
		return ErrAlreadyVerified
	default:
		return errors.New("unknown verification request status")
	}
}

func New(
	userId string,
	answers []Answer,
	contacts string,
) *Verification {
	return &Verification{
		Id:               "",
		UserId:           userId,
		Answers:          []Answer{},
		Contacts:         contacts,
		Status:           VerificationStatusPending,
		RejectionReason:  "",
		VerificationCode: "",
		UpdatedAt:        time.Now(),
		CreatedAt:        time.Now(),
	}
}

// Approve approves the verification.
func (v *Verification) Approve() {
	v.Status = VerificationStatusApproved
	v.UpdatedAt = time.Now()
}

// Reject rejects the verification.
func (v *Verification) Reject(reason string) {
	v.Status = VerificationStatusRejected
	v.RejectionReason = reason
	v.UpdatedAt = time.Now()
}

// Verify verifies the verification.
func (v *Verification) Verify(code string) {
	if code != v.VerificationCode {
		return
	}
	v.Status = VerificationStatusVerified
	v.UpdatedAt = time.Now()
}
