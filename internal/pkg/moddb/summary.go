package moddb

import (
	"html"
	"regexp"
	"strings"
)

// MaxSummaryRunes bounds the description the site renders next to a mod.
const MaxSummaryRunes = 200

var (
	tagPattern   = regexp.MustCompile("(?s)<[^>]*>")
	spacePattern = regexp.MustCompile(`\s+`)
)

// Summary turns ModDB's HTML text into the one plain sentence the site shows:
// tags off, entities resolved, whitespace collapsed, cut at the first full stop
// and then at MaxSummaryRunes characters.
//
// The text is stored and rendered as plain text, so nothing else is escaped or
// stripped.
func Summary(text string) string {
	plain := tagPattern.ReplaceAllString(text, " ")
	plain = html.UnescapeString(plain)
	plain = strings.TrimSpace(spacePattern.ReplaceAllString(plain, " "))

	if stop := strings.Index(plain, "."); stop >= 0 {
		plain = plain[:stop+1]
	}

	return truncate(plain, MaxSummaryRunes)
}

// truncate cuts s to limit runes, never splitting one in half.
func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}

	return strings.TrimSpace(string(runes[:limit]))
}
