package moddb

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// host is the only site a lookup query may name.
const host = "mods.vintagestory.at"

// keyPattern is what ModDB's API accepts as a key: a numeric asset id or a
// modidstr ("carryon", "stone-quarry"). A dot is not part of either, which is
// what makes a pasted domain fall through as a bad reference instead of being
// looked up as a mod.
var keyPattern = regexp.MustCompile("^[A-Za-z0-9_-]+$")

// ParseRef turns what an admin typed into the key that
// mods.vintagestory.at/api/mod/{key} takes. The key is a url alias or a mod id.
//
// The admin panel promises these shapes:
//
//	https://mods.vintagestory.at/carryon -> carryon
//	carryon                              -> carryon
//	890                                  -> 890   (Carry On's mod id)
//
// A page link of the form /show/mod/{N} is refused rather than resolved, because
// its N is an asset id and the API takes a mod id: /show/mod/4405 is Carry On's
// page, while /api/mod/4405 answers Goblin Voices. ModDB offers no lookup by
// asset id, so passing the number on would store a different mod than the one
// the admin was looking at and report no error. See ErrAssetIDRef.
//
// Anything else — an empty query, a link to another site, a path ModDB does not
// serve — is ErrInvalidRef, so a typo is answered at once instead of with a
// lookup that cannot succeed.
func ParseRef(query string) (string, error) {
	text := strings.TrimSpace(query)
	if text == "" {
		return "", ErrInvalidRef
	}

	// A bare reference, with or without the scheme an admin left out when
	// copying a link's host.
	if !strings.Contains(text, "://") {
		if !strings.ContainsAny(text, "/") {
			return bareRef(text)
		}

		text = "https://" + text
	}

	parsed, err := url.Parse(text)
	if err != nil {
		return "", ErrInvalidRef
	}
	if !strings.EqualFold(parsed.Host, host) {
		return "", ErrInvalidRef
	}

	segments := pathSegments(parsed.Path)

	switch {
	case len(segments) == 1:
		return bareRef(segments[0])
	case len(segments) == 3 && segments[0] == "show" && segments[1] == "mod":
		// The number here is an asset id, and /api/mod/{n} resolves a mod id. The
		// two name different mods, and a lookup that returned one would store the
		// wrong mod as required and say nothing about it.
		if _, err := bareRef(segments[2]); err != nil {
			return "", ErrInvalidRef
		}

		return "", ErrAssetIDRef
	default:
		return "", ErrInvalidRef
	}
}

// PageURL is the mod's page on ModDB: its alias when it has one, and the
// numeric asset page otherwise. It is a link for a reader to open, not a
// lookup key: the asset form is what ParseRef refuses.
func PageURL(urlAlias string, assetID int) string {
	if strings.TrimSpace(urlAlias) != "" {
		return BaseURL + "/" + urlAlias
	}

	return fmt.Sprintf("%s/show/mod/%d", BaseURL, assetID)
}

// bareRef accepts a path segment as a ModDB key.
func bareRef(segment string) (string, error) {
	segment = strings.TrimSpace(segment)
	if !keyPattern.MatchString(segment) {
		return "", ErrInvalidRef
	}

	return segment, nil
}

// pathSegments drops the empty pieces a leading or trailing slash leaves.
func pathSegments(path string) []string {
	var segments []string

	for segment := range strings.SplitSeq(path, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}

	return segments
}
