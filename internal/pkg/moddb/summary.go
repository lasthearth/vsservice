package moddb

import (
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxSummaryRunes bounds the description the site renders next to a mod.
const MaxSummaryRunes = 200

var (
	tagPattern = regexp.MustCompile("(?s)<[^>]*>")
	// \s is ASCII only, so the no-break space an &nbsp; resolves to and the
	// zero-width space real descriptions open with are named as well: left
	// alone they survive into the stored text and eat the rune budget.
	spacePattern = regexp.MustCompile(`[\s\x{00A0}\x{200B}]+`)
	// A stripped inline tag leaves a space where it stood, so a sentence that
	// ended in a link reads "...CarryCapacity ." without this.
	spaceBeforeStop = regexp.MustCompile(`\s+\.`)
)

// Summary turns ModDB's HTML text into the one plain sentence the site shows:
// tags off, entities resolved, whitespace collapsed, cut at the end of the
// first sentence and then at MaxSummaryRunes characters.
//
// The text is stored and rendered as plain text, so nothing else is escaped or
// stripped.
func Summary(text string) string {
	plain := tagPattern.ReplaceAllString(text, " ")
	plain = html.UnescapeString(plain)
	plain = strings.TrimSpace(spacePattern.ReplaceAllString(plain, " "))
	plain = spaceBeforeStop.ReplaceAllString(plain, ".")

	if end := sentenceEnd(plain); end >= 0 {
		plain = strings.TrimSpace(plain[:end])
	}

	return truncate(plain, MaxSummaryRunes)
}

// sentenceEnd is the index just past the first full stop that ends a sentence,
// or -1 when nothing does.
//
// A stop has to be followed by a space or by the end of the text. Cutting at
// the first period byte instead stored "1." as the whole description of every
// mod whose text opens with a version number, and "Adds new blocks, e." for one
// that abbreviates.
func sentenceEnd(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] != '.' {
			continue
		}
		if i+1 == len(s) {
			return i + 1
		}
		if next, _ := utf8.DecodeRuneInString(s[i+1:]); unicode.IsSpace(next) {
			return i + 1
		}
	}

	return -1
}

// truncate cuts s to limit runes, never splitting one in half.
func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}

	return strings.TrimSpace(string(runes[:limit]))
}
