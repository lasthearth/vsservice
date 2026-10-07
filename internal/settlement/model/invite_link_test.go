package model

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var inviteNow = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func TestNewInviteLinkValidates(t *testing.T) {
	cases := []struct {
		name    string
		ttl     time.Duration
		maxUses int32
		want    error
	}{
		{"negative ttl", -time.Hour, 0, ErrInviteTTLInvalid},
		{"ttl over 30 days", InviteLinkMaxTTL + time.Second, 0, ErrInviteTTLInvalid},
		{"negative uses", time.Hour, -1, ErrInviteMaxUsesInvalid},
		{"too many uses", time.Hour, InviteLinkMaxUses + 1, ErrInviteMaxUsesInvalid},
	}
	for _, c := range cases {
		if _, err := NewInviteLink("s1", "u1", c.ttl, c.maxUses, inviteNow); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
	}
}

func TestNewInviteLinkCode(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		link, err := NewInviteLink("s1", "u1", time.Hour, 5, inviteNow)
		if err != nil {
			t.Fatal(err)
		}
		if len(link.Code) != inviteCodeLen {
			t.Fatalf("code length %d", len(link.Code))
		}
		for _, r := range link.Code {
			if !strings.ContainsRune(inviteCodeAlphabet, r) {
				t.Fatalf("code %q has %q outside the alphabet", link.Code, r)
			}
		}
		if seen[link.Code] {
			t.Fatalf("duplicate code %q", link.Code)
		}
		seen[link.Code] = true
	}
}

func TestInviteLinkNoExpiryWhenTTLZero(t *testing.T) {
	link, err := NewInviteLink("s1", "u1", 0, 0, inviteNow)
	if err != nil {
		t.Fatal(err)
	}
	if link.ExpiresAt != nil {
		t.Fatalf("want no expiry, got %v", link.ExpiresAt)
	}
	if got := link.Status(inviteNow.Add(365 * 24 * time.Hour)); got != InviteLinkActive {
		t.Fatalf("unlimited link should stay active, got %s", got)
	}
}

func TestInviteLinkUseCountsAndExhausts(t *testing.T) {
	link, _ := NewInviteLink("s1", "u1", time.Hour, 2, inviteNow)

	for i := range 2 {
		if err := link.Use(inviteNow); err != nil {
			t.Fatalf("use %d: %v", i+1, err)
		}
	}
	if err := link.Use(inviteNow); !errors.Is(err, ErrInviteExhausted) {
		t.Fatalf("third use: want ErrInviteExhausted, got %v", err)
	}
	if link.Uses != 2 {
		t.Fatalf("a refused use must not count, uses=%d", link.Uses)
	}
	if link.Status(inviteNow) != InviteLinkExhausted {
		t.Fatalf("want exhausted, got %s", link.Status(inviteNow))
	}
}

func TestInviteLinkExpires(t *testing.T) {
	link, _ := NewInviteLink("s1", "u1", time.Hour, 0, inviteNow)

	if err := link.Use(inviteNow.Add(59 * time.Minute)); err != nil {
		t.Fatalf("before expiry: %v", err)
	}
	if err := link.Use(inviteNow.Add(time.Hour)); !errors.Is(err, ErrInviteExpired) {
		t.Fatalf("at expiry: want ErrInviteExpired, got %v", err)
	}
}

func TestInviteLinkRevokeWinsAndKeepsFirstMoment(t *testing.T) {
	link, _ := NewInviteLink("s1", "u1", time.Hour, 1, inviteNow)
	_ = link.Use(inviteNow)

	link.Revoke(inviteNow)
	link.Revoke(inviteNow.Add(time.Minute))

	if !link.RevokedAt.Equal(inviteNow) {
		t.Fatalf("second revoke moved the moment to %v", link.RevokedAt)
	}
	if got := link.Status(inviteNow.Add(2 * time.Hour)); got != InviteLinkRevoked {
		t.Fatalf("revoked must win over expired and exhausted, got %s", got)
	}
	if err := link.Use(inviteNow); !errors.Is(err, ErrInviteRevoked) {
		t.Fatalf("want ErrInviteRevoked, got %v", err)
	}
}
