package model

import (
	"regexp"
	"strings"
)

// MaxSlugLength bounds a slug so it stays a usable URL segment.
const MaxSlugLength = 80

// translit maps Russian letters to their Latin counterparts, following the
// same table the editors use, so a slug typed on the site and one derived on
// the server never disagree.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

// nonSlugChars matches everything a slug must not contain.
var nonSlugChars = regexp.MustCompile("[^a-z0-9]+")

// validSlug matches a slug: lowercase latin words joined by single dashes.
var validSlug = regexp.MustCompile("^[a-z0-9]+(?:-[a-z0-9]+)*$")

// Slugify derives a URL segment from a heading.
//
// Slugify("Как объявить набег") returns "kak-obyavit-nabeg". The result can be
// empty when the heading has no letters or digits at all.
func Slugify(text string) string {
	var b strings.Builder

	for _, r := range strings.ToLower(text) {
		if latin, ok := translit[r]; ok {
			b.WriteString(latin)
			continue
		}

		b.WriteRune(r)
	}

	slug := strings.Trim(nonSlugChars.ReplaceAllString(b.String(), "-"), "-")
	if len(slug) > MaxSlugLength {
		slug = strings.Trim(slug[:MaxSlugLength], "-")
	}

	return slug
}

// IsValidSlug reports whether a string can serve as an article slug.
func IsValidSlug(slug string) bool {
	return len(slug) <= MaxSlugLength && validSlug.MatchString(slug)
}
