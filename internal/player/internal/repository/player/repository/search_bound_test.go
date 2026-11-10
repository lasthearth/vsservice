package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	pkgierror "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/regexsafe"
	"google.golang.org/grpc/codes"
)

// TestBoundedSearchQuery covers the bound itself: the width in runes is what is
// counted, the byte check only answers an oversized body before it is counted,
// and every legal width passes.
func TestBoundedSearchQuery(t *testing.T) {
	cases := map[string]struct {
		query   string
		wantErr bool
	}{
		"empty":                   {"", false},
		"one rune":                {"а", false},
		"32 ascii runes":          {strings.Repeat("a", 32), false},
		"32 cyrillic runes":       {strings.Repeat("а", 32), false},
		"32 four-byte runes":      {strings.Repeat("🙂", 32), false},
		"32 metacharacters":       {strings.Repeat("(a*)", 8), false},
		"33 ascii runes":          {strings.Repeat("a", 33), true},
		"33 cyrillic runes":       {strings.Repeat("а", 33), true},
		"33 four-byte runes":      {strings.Repeat("🙂", 33), true},
		"33 metacharacters":       {strings.Repeat("(a*)", 8) + "b", true},
		"one mebibyte of pattern": {strings.Repeat("(a*)*b", 1<<20/6), true},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := boundedSearchQuery(tc.query)

			if !tc.wantErr {
				if err != nil {
					t.Fatalf("boundedSearchQuery on a %d rune query: %v", len([]rune(tc.query)), err)
				}
				return
			}

			if !errors.Is(err, errQueryTooLong) {
				t.Fatalf("err = %v, want the bound reported", err)
			}
		})
	}
}

// TestBoundedSearchQueryAnswersInvalidArgument pins the code the caller sees:
// the service returns the repository error as it is, so the bound has to carry
// the argument failure itself.
func TestBoundedSearchQueryAnswersInvalidArgument(t *testing.T) {
	err := boundedSearchQuery(strings.Repeat("a", maxSearchQueryRunes+1))

	var domainErr *pkgierror.DomainError
	if !errors.As(err, &domainErr) {
		t.Fatalf("err = %v (%T), want a domain error", err, err)
	}
	if domainErr.Code != codes.InvalidArgument {
		t.Errorf("code = %v, want %v", domainErr.Code, codes.InvalidArgument)
	}
	if !strings.Contains(domainErr.Message, "32") {
		t.Errorf("message = %q, want it to name the bound", domainErr.Message)
	}
}

// TestSearchUsersRefusesAnOverLongQueryBeforeTheDriver is the regression the
// audit named. The repository has no collection here, so a query that reached
// the driver would panic on the nil handle instead of returning: a clean error
// is the proof that the bound ran first and that no oversized term was ever
// escaped or sent to MongoDB.
func TestSearchUsersRefusesAnOverLongQueryBeforeTheDriver(t *testing.T) {
	var repo Repository

	for name, query := range map[string]string{
		"one rune over":     strings.Repeat("а", maxSearchQueryRunes+1),
		"one mebibyte":      strings.Repeat("(a*)*b", 1<<20/6),
		"grpc receive size": strings.Repeat(".", 60<<20/16),
	} {
		t.Run(name, func(t *testing.T) {
			users, err := repo.SearchUsers(context.Background(), query)
			if !errors.Is(err, errQueryTooLong) {
				t.Fatalf("err = %v, want the bound reported for a %d byte query", err, len(query))
			}
			if users != nil {
				t.Errorf("users = %v, want none", users)
			}
		})
	}
}

// TestSearchUsersBoundMatchesTheRosterPrecedent keeps the two name lookups on
// one number: the roster refuses a query over 32 runes in the service
// (internal/player/internal/service/player/list.go), and SearchUsers is the same
// kind of lookup. It also pins what the bound buys - the widest legal query
// escapes to at most twice its runes, so the pattern MongoDB receives is bounded
// by the bound.
func TestSearchUsersBoundMatchesTheRosterPrecedent(t *testing.T) {
	if maxSearchQueryRunes != 32 {
		t.Errorf("maxSearchQueryRunes = %d, want 32 as in the roster bound", maxSearchQueryRunes)
	}

	widest := strings.Repeat("(", maxSearchQueryRunes)
	if got := len([]rune(regexsafe.Escape(widest))); got > 2*maxSearchQueryRunes {
		t.Errorf("the widest legal query escaped to %d runes, want at most %d", got, 2*maxSearchQueryRunes)
	}
}
