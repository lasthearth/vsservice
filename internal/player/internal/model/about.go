package model

import (
	"strings"
	"time"
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

	// maxAboutBytes is the largest byte length a text of MaxAboutRunes runes can
	// have. A longer body is refused before it is copied, split per line and
	// rewritten, so one oversized request cannot turn into hundreds of megabytes
	// of work on the way to an answer that says "too long".
	maxAboutBytes = MaxAboutRunes * utf8.UTFMax
)

// MaxReasonRunes bounds a moderator's reason for hiding a description. The
// reason is stored in a notification the player reads and in the audit log, so
// it cannot be a document.
const MaxReasonRunes = 500

// About errors are domain errors, so the interceptor answers 400 for them
// without the service having to name a gRPC code.
var (
	ErrAboutTooLong           = ierror.InvalidArgument("about is longer than 500 characters")
	ErrAboutTooManyLines      = ierror.InvalidArgument("about has more than 8 lines")
	ErrAboutControlCharacters = ierror.InvalidArgument("about contains control characters")
)

// isInvisible reports whether r renders as nothing but changes how the text
// around it reads: the direction marks and embeddings (U+200E, U+200F, U+061C,
// U+202A..U+202E, U+2066..U+2069), the soft hyphen, the zero-width spaces and
// the byte-order mark. A description carrying one reads differently from the
// bytes a moderator inspecting it sees, which is what a spoofed nickname next
// to it needs.
//
// unicode.IsControl covers none of these: it reports the C0 and C1 range only,
// so the whole Cf category has to be named. The two joiners stay, because an
// emoji sequence and several scripts need them and they carry no direction.
func isInvisible(r rune) bool {
	if r == 0x200C || r == 0x200D { // ZWNJ and ZWJ
		return false
	}

	return unicode.In(r, unicode.Cf)
}

// NormalizeAbout prepares a description for storage, or reports why it cannot
// be stored. An empty result means "no description".
//
// The text is stored and rendered as plain text: nothing is escaped and no
// markup is stripped, because the site interpolates it.
func NormalizeAbout(raw string) (string, error) {
	// Refuse an oversized body before doing any work on it. Every byte beyond
	// the limit is copied, split and rewritten otherwise, only for the rune count
	// at the end to reject it.
	if len(raw) > maxAboutBytes {
		return "", ErrAboutTooLong
	}

	// Line endings first, so every later step sees one newline character. The
	// two Unicode line separators join them: a browser renders U+2028 and U+2029
	// as a line break, so keeping them would let a description bypass the line
	// bound and still fill the card it is shown in.
	replaced := strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\n",
		"\u2028", "\n",
		"\u2029", "\n",
		"\t", " ",
	).Replace(raw)

	var b strings.Builder

	for _, r := range replaced {
		if r != '\n' && (unicode.IsControl(r) || isInvisible(r)) {
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

// SanitizeReason prepares a moderator's reason for storage. It is shown to the
// player and written to the audit log, and the two must read the same, so the
// characters a description is refused for are dropped from it rather than
// refused: the moderator is not the one who has to fix the text.
func SanitizeReason(raw string) string {
	if len(raw) > MaxReasonRunes*utf8.UTFMax {
		raw = raw[:MaxReasonRunes*utf8.UTFMax]
	}

	replaced := strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", " ").Replace(raw)

	var b strings.Builder

	for _, r := range replaced {
		if r == '\n' || (!unicode.IsControl(r) && !isInvisible(r)) {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// SetAbout records a description and the time it was written on the player that
// owns it. A nil at is the stamp of a removal, which is what an empty text
// writes.
func (p *Player) SetAbout(about string, at *time.Time) {
	p.About = about
	p.AboutUpdatedAt = at
}
