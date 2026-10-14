package model

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeAbout(t *testing.T) {
	full := strings.Repeat("я", MaxAboutRunes)
	eightLines := strings.Repeat("строка\n", MaxAboutLines-1) + "строка"
	nineLines := strings.Repeat("строка\n", MaxAboutLines) + "строка"

	cases := []struct {
		name string
		in   string
		want string
		err  error
	}{
		{name: "nothing written", in: "", want: ""},
		{name: "whitespace only is no description", in: "   \n\t\n  ", want: ""},
		{name: "crlf becomes lf", in: "первая\r\nвторая", want: "первая\nвторая"},
		{name: "a bare cr becomes lf", in: "первая\rвторая", want: "первая\nвторая"},
		{name: "a tab becomes a space", in: "первая\tвторая", want: "первая вторая"},
		{name: "trailing spaces are cut from every line", in: "первая   \nвторая  ", want: "первая\nвторая"},
		{name: "runs of empty lines collapse into one", in: "первая\n\n\n\n\nвторая", want: "первая\n\nвторая"},
		{name: "exactly the limit passes", in: full, want: full},
		{name: "one rune over the limit fails", in: full + "я", err: ErrAboutTooLong},
		// The byte bound is what an oversized body hits first, before the text is
		// copied, split per line and rewritten on the way to being refused.
		{name: "a body over the byte bound fails before any work",
			in:  strings.Repeat("я", maxAboutBytes),
			err: ErrAboutTooLong},
		{name: "eight lines pass", in: eightLines, want: eightLines},
		{name: "nine lines fail", in: nineLines, err: ErrAboutTooManyLines},
		{name: "a control character fails", in: "привет\x07", err: ErrAboutControlCharacters},
		{name: "a text direction mark fails", in: "привет\u202E", err: ErrAboutControlCharacters},
		{name: "a right-to-left mark fails", in: "admin\u200F", err: ErrAboutControlCharacters},
		{name: "a soft hyphen fails", in: "при\u00ADвет", err: ErrAboutControlCharacters},
		{name: "a byte-order mark fails", in: "\uFEFFпривет", err: ErrAboutControlCharacters},
		{name: "an emoji sequence keeps the joiners it needs",
			in:   "семья \U0001F468\u200D\U0001F469\u200D\U0001F467",
			want: "семья \U0001F468\u200D\U0001F469\u200D\U0001F467"},
		{name: "a Unicode line separator becomes a line", in: "раз\u2028два", want: "раз\nдва"},
		{name: "nine separator lines fail",
			in:  strings.Repeat("строка\u2028", MaxAboutLines) + "строка",
			err: ErrAboutTooManyLines},
		{name: "markup stays plain text", in: "<b>жирный</b>", want: "<b>жирный</b>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeAbout(tc.in)

			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("NormalizeAbout err = %v, want %v", err, tc.err)
				}
				if got != "" {
					t.Errorf("NormalizeAbout kept %q on an error", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("NormalizeAbout err = %v, want nil", err)
			}
			if got != tc.want {
				t.Errorf("NormalizeAbout = %q, want %q", got, tc.want)
			}
		})
	}
}
