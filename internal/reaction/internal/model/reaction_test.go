package model

import (
	"fmt"
	"slices"
	"testing"
)

func TestValidateTarget(t *testing.T) {
	for _, ok := range []string{"news:66f0c0ffee", "diplomacy:1234567890123456789", "event:a_b-c"} {
		if err := ValidateTarget(ok); err != nil {
			t.Errorf("%q: unexpected error %v", ok, err)
		}
	}
	for _, bad := range []string{"", "news:", "news", "user:1", "news:a b", "news:a/b", "NEWS:1"} {
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

	if _, err := ValidateTargets(nil); err != ErrNoTargets {
		t.Errorf("want ErrNoTargets, got %v", err)
	}

	many := make([]string, MaxTargets+1)
	for i := range many {
		many[i] = fmt.Sprintf("news:%d", i)
	}
	if _, err := ValidateTargets(many); err != ErrTooManyTargets {
		t.Errorf("want ErrTooManyTargets, got %v", err)
	}

	if _, err := ValidateTargets([]string{"news:a", "bad"}); err != ErrInvalidTarget {
		t.Errorf("want ErrInvalidTarget, got %v", err)
	}
}
