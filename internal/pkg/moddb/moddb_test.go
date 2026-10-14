package moddb

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
		err   error
	}{
		{
			name:  "an alias link",
			query: "https://mods.vintagestory.at/carryon",
			want:  "carryon",
		},
		{
			// A page link carries an asset id, and /api/mod/{n} resolves a mod
			// id: /show/mod/4405 is Carry On's page while /api/mod/4405 answers
			// Goblin Voices. Resolving it would store the wrong mod.
			name:  "a page link is refused, not resolved to another mod",
			query: "https://mods.vintagestory.at/show/mod/4405",
			err:   ErrAssetIDRef,
		},
		{
			name:  "a bare alias",
			query: "  carryon  ",
			want:  "carryon",
		},
		{
			// A bare number is a mod id, which is what the API takes.
			name:  "a bare mod id",
			query: "890",
			want:  "890",
		},
		{
			name:  "a mod id with a dash",
			query: "stone-quarry",
			want:  "stone-quarry",
		},
		{
			name:  "a link without the scheme",
			query: "mods.vintagestory.at/carryon",
			want:  "carryon",
		},
		{
			name:  "a link with a trailing slash",
			query: "https://mods.vintagestory.at/carryon/",
			want:  "carryon",
		},
		{
			name:  "empty",
			query: "   ",
			err:   ErrInvalidRef,
		},
		{
			name:  "another site",
			query: "https://example.com/carryon",
			err:   ErrInvalidRef,
		},
		{
			name:  "another site without the scheme",
			query: "example.com/carryon",
			err:   ErrInvalidRef,
		},
		{
			name:  "a bare domain is not a mod",
			query: "example.com",
			err:   ErrInvalidRef,
		},
		{
			name:  "a path ModDB does not serve",
			query: "https://mods.vintagestory.at/api/mod/carryon",
			err:   ErrInvalidRef,
		},
		{
			name:  "a show path without a mod",
			query: "https://mods.vintagestory.at/show/mod",
			err:   ErrInvalidRef,
		},
		{
			name:  "a page link whose number is not a number",
			query: "https://mods.vintagestory.at/show/mod/carryon!",
			err:   ErrInvalidRef,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRef(tc.query)

			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("ParseRef(%q) err = %v, want %v", tc.query, err, tc.err)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseRef(%q) err = %v, want nil", tc.query, err)
			}
			if got != tc.want {
				t.Errorf("ParseRef(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

func TestSummary(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			name: "tags come off and the first sentence stays",
			text: "<p>Carry On lets you pick up placed blocks.</p><p>More about that later.</p>",
			want: "Carry On lets you pick up placed blocks.",
		},
		{
			name: "entities are resolved",
			text: "<p>Carry &amp; move &quot;containers&quot; with their contents.</p>",
			want: "Carry & move \"containers\" with their contents.",
		},
		{
			name: "a line break inside a tag becomes a space",
			text: "<p>First line<br/>second line.</p>",
			want: "First line second line.",
		},
		{
			name: "a description without a full stop is kept whole",
			text: "<p>No punctuation here</p>",
			want: "No punctuation here",
		},
		{
			// A version number opens a great many ModDB descriptions. Cutting at
			// the first period byte stored "1." as the whole summary of a mod
			// whose text opens "1.17.9 Patched an Exploit...".
			name: "a period inside a version number ends no sentence",
			text: "<p>1.17.9 Patched an exploit. Also fixed a crash.</p>",
			want: "1.17.9 Patched an exploit.",
		},
		{
			// An abbreviation still reads as a sentence end. What matters is
			// that the cut lands on a boundary rather than inside a token.
			name: "an abbreviation ends the sentence early but whole",
			text: "<p>Adds new blocks, e.g. quartz and marble.</p>",
			want: "Adds new blocks, e.g.",
		},
		{
			// Carry On's real description opens with a link, and a stripped
			// inline tag leaves a space where it stood.
			name: "a sentence that ended in a link keeps no stray space",
			text: "<p>Fork of the mod by copygirl - <a href=\"https://mods.vintagestory.at/show/mod/38\">CarryCapacity</a>. Carry On lets you pick up blocks.</p>",
			want: "Fork of the mod by copygirl - CarryCapacity.",
		},
		{
			// ModDB descriptions open with a zero-width space and indent with
			// no-break space runs, neither of which \s matches.
			name: "a zero-width and a no-break space are whitespace",
			text: "<p>\u200bNote:\u00a0v5 is out.</p>",
			want: "Note: v5 is out.",
		},
		{
			name: "empty",
			text: "",
			want: "",
		},
		{
			name: "markup only",
			text: "<div><br/></div>",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Summary(tc.text); got != tc.want {
				t.Errorf("Summary(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestSummaryIsBounded(t *testing.T) {
	long := strings.Repeat("я", MaxSummaryRunes+50)

	got := Summary(long)

	if runes := []rune(got); len(runes) != MaxSummaryRunes {
		t.Errorf("Summary kept %d runes, want %d", len(runes), MaxSummaryRunes)
	}
}

func TestSummaryStopsAtTheFirstFullStopBeforeTheLimit(t *testing.T) {
	got := Summary("Коротко. " + strings.Repeat("дальше ", 100))

	if got != "Коротко." {
		t.Errorf("Summary = %q, want the first sentence", got)
	}
}

func TestReleaseReleasedAt(t *testing.T) {
	r := Release{Created: "2026-10-14 13:43:04"}

	want := "2026-10-14T13:43:04Z"
	if got := r.ReleasedAt().Format("2006-01-02T15:04:05Z"); got != want {
		t.Errorf("ReleasedAt = %s, want %s", got, want)
	}

	broken := Release{Created: "not a date"}
	if !broken.ReleasedAt().IsZero() {
		t.Error("an unparsable stamp must be the zero time")
	}
}
