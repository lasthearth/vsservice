// Package regexsafe turns caller-supplied text into a MongoDB $regex pattern
// without letting that text become pattern syntax.
package regexsafe

import "strings"

// Escape neutralizes regex metacharacters in a search string, so a query is
// only ever matched literally and cannot be used to build a pattern.
func Escape(value string) string {
	var b []rune

	for _, r := range value {
		if strings.ContainsRune(`.*+?^$()[]{}|\\`, r) {
			b = append(b, '\\')
		}

		b = append(b, r)
	}

	return string(b)
}
