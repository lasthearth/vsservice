package model

import (
	"errors"
	"testing"
)

func TestMailClaimStateMachine(t *testing.T) {
	// unread → read → claimed
	c := NewMailClaim("m1", "p1")
	if c.State != MailStateUnread {
		t.Fatalf("fresh claim state = %q, want unread", c.State)
	}

	if err := c.MarkRead(); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if c.State != MailStateRead || c.ReadAt == nil {
		t.Fatalf("after MarkRead: state=%q readAt=%v", c.State, c.ReadAt)
	}

	// MarkRead is idempotent.
	if err := c.MarkRead(); err != nil {
		t.Fatalf("second MarkRead: %v", err)
	}
	if c.State != MailStateRead {
		t.Fatalf("second MarkRead changed state to %q", c.State)
	}

	if err := c.Claim(true); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if c.State != MailStateClaimed || c.ClaimedAt == nil {
		t.Fatalf("after Claim: state=%q claimedAt=%v", c.State, c.ClaimedAt)
	}

	// Claim is idempotent, MarkRead on a claimed row is a no-op.
	if err := c.Claim(true); err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if err := c.MarkRead(); err != nil {
		t.Fatalf("MarkRead after claim: %v", err)
	}
	if c.State != MailStateClaimed {
		t.Fatalf("claimed row mutated to %q", c.State)
	}
}

func TestClaimNotification(t *testing.T) {
	c := NewMailClaim("m1", "p1")
	// A notification (no attachments) cannot be claimed.
	if err := c.Claim(false); !errors.Is(err, ErrNothingToClaim) {
		t.Fatalf("Claim(false) = %v, want ErrNothingToClaim", err)
	}
	if c.State != MailStateUnread {
		t.Fatalf("failed claim mutated state to %q", c.State)
	}
}

func TestClaimFromUnreadStampsRead(t *testing.T) {
	// Claiming straight from unread should also record ReadAt.
	c := NewMailClaim("m1", "p1")
	if err := c.Claim(true); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if c.ReadAt == nil {
		t.Fatal("Claim from unread did not stamp ReadAt")
	}
}
