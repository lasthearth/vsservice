package verification_test

import (
	"errors"
	"testing"

	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
)

func TestVerification_CanSubmit(t *testing.T) {
	tests := []struct {
		name       string
		status     verification.Status
		wantErr    error
		wantNonNil bool
	}{
		{"rejected allows resubmit", verification.VerificationStatusRejected, nil, false},
		{"pending blocks resubmit", verification.VerificationStatusPending, verification.ErrVerificationPending, false},
		{"approved blocks resubmit", verification.VerificationStatusApproved, verification.ErrAlreadyVerified, false},
		{"verified blocks resubmit", verification.VerificationStatusVerified, verification.ErrAlreadyVerified, false},
		{"unknown/zero status returns error", verification.Status(""), nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &verification.Verification{Status: tc.status}
			err := v.CanSubmit()
			if tc.wantNonNil {
				if err == nil {
					t.Errorf("CanSubmit() = nil, want non-nil error")
				}
			} else {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("CanSubmit() = %v, want %v", err, tc.wantErr)
				}
			}
		})
	}
}

func TestVerification_Approve(t *testing.T) {
	tests := []struct {
		name    string
		status  verification.Status
		wantErr error
	}{
		{"pending can be approved", verification.VerificationStatusPending, nil},
		{"approved cannot be re-approved", verification.VerificationStatusApproved, verification.ErrInvalidTransition},
		{"verified cannot be approved", verification.VerificationStatusVerified, verification.ErrInvalidTransition},
		{"rejected cannot be approved", verification.VerificationStatusRejected, verification.ErrInvalidTransition},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &verification.Verification{Status: tc.status}
			err := v.Approve()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Approve() error = %v, want %v", err, tc.wantErr)
			}
			if err == nil && v.Status != verification.VerificationStatusApproved {
				t.Errorf("Approve() status = %v, want approved", v.Status)
			}
		})
	}
}

func TestVerification_Reject(t *testing.T) {
	tests := []struct {
		name    string
		status  verification.Status
		reason  string
		wantErr error
	}{
		{"pending can be rejected", verification.VerificationStatusPending, "bad answers", nil},
		{"approved cannot be rejected", verification.VerificationStatusApproved, "too late", verification.ErrInvalidTransition},
		{"verified cannot be rejected", verification.VerificationStatusVerified, "too late", verification.ErrInvalidTransition},
		{"rejected cannot be re-rejected", verification.VerificationStatusRejected, "already", verification.ErrInvalidTransition},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &verification.Verification{Status: tc.status}
			err := v.Reject(tc.reason)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Reject() error = %v, want %v", err, tc.wantErr)
			}
			if err == nil {
				if v.Status != verification.VerificationStatusRejected {
					t.Errorf("Reject() status = %v, want rejected", v.Status)
				}
				if v.RejectionReason != tc.reason {
					t.Errorf("Reject() reason = %v, want %v", v.RejectionReason, tc.reason)
				}
			}
		})
	}
}

func TestVerification_Verify(t *testing.T) {
	tests := []struct {
		name      string
		status    verification.Status
		stored    string
		inputCode string
		wantErr   error
	}{
		{"approved with correct code verifies", verification.VerificationStatusApproved, "ABC123", "ABC123", nil},
		{"approved with wrong code errors", verification.VerificationStatusApproved, "ABC123", "WRONG", verification.ErrInvalidVerificationCode},
		{"approved with empty code errors", verification.VerificationStatusApproved, "ABC123", "", verification.ErrInvalidVerificationCode},
		{"pending cannot verify", verification.VerificationStatusPending, "ABC123", "ABC123", verification.ErrInvalidTransition},
		{"verified cannot verify again", verification.VerificationStatusVerified, "ABC123", "ABC123", verification.ErrInvalidTransition},
		{"rejected cannot verify", verification.VerificationStatusRejected, "ABC123", "ABC123", verification.ErrInvalidTransition},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := &verification.Verification{
				Status:           tc.status,
				VerificationCode: tc.stored,
			}
			err := v.Verify(tc.inputCode)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Verify() error = %v, want %v", err, tc.wantErr)
			}
			if err == nil && v.Status != verification.VerificationStatusVerified {
				t.Errorf("Verify() status = %v, want verified", v.Status)
			}
		})
	}
}
