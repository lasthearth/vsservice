package regexsafe

import "testing"

func TestEscapeNeutralizesMetacharacters(t *testing.T) {
	cases := map[string]string{
		"поселения":   "поселения",
		"a.b":         "a\\.b",
		"a*b":         "a\\*b",
		"(group)":     "\\(group\\)",
		"[class]":     "\\[class\\]",
		"a|b":         "a\\|b",
		"^start$":     "\\^start\\$",
		"back\\slash": "back\\\\slash",
	}

	for input, want := range cases {
		if got := Escape(input); got != want {
			t.Errorf("Escape(%q) = %q, want %q", input, got, want)
		}
	}
}
