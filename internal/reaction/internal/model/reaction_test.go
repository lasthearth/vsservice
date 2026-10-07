package model

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/lasthearth/vsservice/internal/reaction/internal/ierror"
)

func TestValidateTarget(t *testing.T) {
	for _, ok := range []string{"news:66f0c0ffee", "diplomacy:1234567890123456789", "event:a_b-c"} {
		if err := ValidateTarget(ok); err != nil {
			t.Errorf("%q: unexpected error %v", ok, err)
		}
	}
	// The trailing-newline case matters: Go's $ without the m flag is
	// end-of-text, so "event:abc\n" must not slip through as a valid target.
	for _, bad := range []string{"", "news:", "news", "user:1", "news:a b", "news:a/b", "NEWS:1", "event:abc\n"} {
		if err := ValidateTarget(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestValidateEmoji(t *testing.T) {
	if err := ValidateEmoji("fire"); err != nil {
		t.Error(err)
	}
	if err := ValidateEmoji("poop"); err == nil {
		t.Error("want error for unknown emoji")
	}
}

func TestValidateTargets(t *testing.T) {
	got, err := ValidateTargets([]string{"news:a", "news:b", "news:a"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"news:a", "news:b"}) {
		t.Errorf("want deduped in order, got %v", got)
	}

	if _, err := ValidateTargets(nil); !errors.Is(err, ierror.ErrNoTargets) {
		t.Errorf("want ErrNoTargets, got %v", err)
	}

	many := make([]string, MaxTargets+1)
	for i := range many {
		many[i] = fmt.Sprintf("news:%d", i)
	}
	if _, err := ValidateTargets(many); !errors.Is(err, ierror.ErrTooManyTargets) {
		t.Errorf("want ErrTooManyTargets, got %v", err)
	}

	if _, err := ValidateTargets([]string{"news:a", "bad"}); !errors.Is(err, ierror.ErrInvalidTarget) {
		t.Errorf("want ErrInvalidTarget, got %v", err)
	}
}

func TestEmojisAreUniqueKeys(t *testing.T) {
	seen := make(map[string]bool, len(Emojis))
	for _, e := range Emojis {
		if seen[e] {
			t.Errorf("duplicate emoji %q", e)
		}
		seen[e] = true
		if err := ValidateEmoji(e); err != nil {
			t.Errorf("%q should be valid: %v", e, err)
		}
	}
	if len(Emojis) != 48 {
		t.Errorf("want 48 emojis (keep in sync with the site), got %d", len(Emojis))
	}
}
