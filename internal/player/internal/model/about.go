package model

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lasthearth/vsservice/internal/pkg/ierror"
)

// MaxAboutRunes and MaxAboutLines bound a player's description. The site shows
// it in a profile card next to the nickname and in the roster row, so it cannot
// grow into a page.
const (
	MaxAboutRunes = 500
	MaxAboutLines = 8
)

// About errors are domain errors, so the interceptor answers 400 for them
// without the service having to name a gRPC code.
var (
	ErrAboutTooLong           = ierror.InvalidArgument("about is longer than 500 characters")
	ErrAboutTooManyLines      = ierror.InvalidArgument("about has more than 8 lines")
	ErrAboutControlCharacters = ierror.InvalidArgument("about contains control characters")
)

// isBidiControl reports whether r is one of the Unicode text-direction marks and
// embeddings. They reorder the text around them, so a description that carries
// one reads differently from what its author typed; unicode.IsControl does not
// cover them.
func isBidiControl(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}

// NormalizeAbout prepares a description for storage, or reports why it cannot
// be stored. An empty result means "no description".
//
// The text is stored and rendered as plain text: nothing is escaped and no
// markup is stripped, because the site interpolates it.
func NormalizeAbout(raw string) (string, error) {
	// Line endings first, so every later step sees one newline character.
	replaced := strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", " ").Replace(raw)

	var b strings.Builder

	for _, r := range replaced {
		if r != '\n' && (unicode.IsControl(r) || isBidiControl(r)) {
			return "", ErrAboutControlCharacters
		}

		b.WriteRune(r)
	}

	lines := strings.Split(b.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}

	// At most one empty line in a row: three newlines collapse into two.
	about := strings.Join(lines, "\n")
	for strings.Contains(about, "\n\n\n") {
		about = strings.ReplaceAll(about, "\n\n\n", "\n\n")
	}

	about = strings.TrimSpace(about)
	if about == "" {
		return "", nil
	}

	// Runes, not bytes: the site counts characters, and Cyrillic is two bytes
	// each in UTF-8.
	if utf8.RuneCountInString(about) > MaxAboutRunes {
		return "", ErrAboutTooLong
	}

	if strings.Count(about, "\n")+1 > MaxAboutLines {
		return "", ErrAboutTooManyLines
	}

	return about, nil
}
