package model

import (
	"regexp"
	"slices"

	"github.com/lasthearth/vsservice/internal/reaction/internal/ierror"
)

// Emojis are the reactions a player can put, in display order.
var Emojis = []string{"like", "heart", "fire", "laugh", "swords"}

// MaxTargets is how many targets one request may ask about.
const MaxTargets = 50

// targetPattern is "<kind>:<id>". Kinds are the site's reactable content;
// ids are Mongo ObjectIDs or Discord snowflakes.
var targetPattern = regexp.MustCompile(`^(news|diplomacy|event):[A-Za-z0-9_-]{1,64}$`)

// ValidateTarget checks the "<kind>:<id>" shape.
func ValidateTarget(target string) error {
	if !targetPattern.MatchString(target) {
		return ierror.ErrInvalidTarget
	}
	return nil
}

// ValidateEmoji checks the emoji is one of Emojis.
func ValidateEmoji(emoji string) error {
	if !slices.Contains(Emojis, emoji) {
		return ierror.ErrInvalidEmoji
	}
	return nil
}

// ValidateTargets checks a request's target list and returns it without
// duplicates, order kept. Callers that zip the response against their own
// request list must match by target, not by position, because duplicates
// collapse.
func ValidateTargets(targets []string) ([]string, error) {
	if len(targets) == 0 {
		return nil, ierror.ErrNoTargets
	}
	if len(targets) > MaxTargets {
		return nil, ierror.ErrTooManyTargets
	}

	seen := make(map[string]struct{}, len(targets))
	unique := make([]string, 0, len(targets))
	for _, t := range targets {
		if err := ValidateTarget(t); err != nil {
			return nil, err
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		unique = append(unique, t)
	}

	return unique, nil
}

// Count is how many players put an emoji on a target.
type Count struct {
	Target string
	Emoji  string
	Count  int64
}
