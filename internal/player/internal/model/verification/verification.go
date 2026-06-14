package verification

import (
	"time"

	"github.com/go-faster/errors"
)

type Status string

const (
	VerificationStatusPending  Status = "pending"
	VerificationStatusApproved Status = "approved"
	VerificationStatusVerified Status = "verified"
	VerificationStatusRejected Status = "rejected"
)

type Verification struct {
	Id string
	// User id from sso
	UserId           string
	UserName         string
	UserGameName     string
	Answers          []Answer
	Contacts         string
	Status           Status
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
	userId,
	userName,
	userGameName string,
	answers []Answer,
	contacts string,
) *Verification {
	now := time.Now()
	return &Verification{
		Id:               "",
		UserId:           userId,
		UserName:         userName,
		UserGameName:     userGameName,
		Answers:          answers,
		Contacts:         contacts,
		Status:           VerificationStatusPending,
		RejectionReason:  "",
		VerificationCode: "",
		UpdatedAt:        now,
		CreatedAt:        now,
	}
}

// Approve transitions pending → approved. Returns ErrInvalidTransition if not pending.
func (v *Verification) Approve() error {
	if v.Status != VerificationStatusPending {
		return ErrInvalidTransition
	}
	v.Status = VerificationStatusApproved
	v.UpdatedAt = time.Now()
	return nil
}

// Reject transitions pending → rejected. Returns ErrInvalidTransition if not pending.
func (v *Verification) Reject(reason string) error {
	if v.Status != VerificationStatusPending {
		return ErrInvalidTransition
	}
	v.Status = VerificationStatusRejected
	v.RejectionReason = reason
	v.UpdatedAt = time.Now()
	return nil
}

// Verify transitions approved → verified. Returns ErrInvalidTransition if not approved,
// ErrInvalidVerificationCode if code does not match.
func (v *Verification) Verify(code string) error {
	if v.Status != VerificationStatusApproved {
		return ErrInvalidTransition
	}
	if code != v.VerificationCode {
		return ErrInvalidVerificationCode
	}
	v.Status = VerificationStatusVerified
	v.UpdatedAt = time.Now()
	return nil
}
