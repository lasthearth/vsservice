package regexsafe

import (
	"math/rand/v2"
	"runtime"
	"strings"
	"testing"
)

// sink keeps the compiler from dropping a call whose only result is its cost.
var sink string

// escapeRunes is the shipped form: an intermediate []rune accumulator. It stays
// here as the oracle the strings.Builder form is compared against, so identical
// output is a checked property and not a claim in a comment.
func escapeRunes(value string) string {
	var b []rune

	for _, r := range value {
		if strings.ContainsRune(`.*+?^$()[]{}|\\`, r) {
			b = append(b, '\\')
		}

		b = append(b, r)
	}

	return string(b)
}

// mixedInput builds a deterministic string of the given byte length out of
// metacharacters, ASCII, Cyrillic, a four-byte rune and bytes that are not
// UTF-8 at all, so one input covers every width Escape has to handle.
func mixedInput(length int) string {
	alphabet := []string{
		"a", "*", ".", "(", ")", "[", "]", "{", "}", "|", `\`, "^", "$", "+", "?",
		"осада", "ве", "\u4e2d", "\U0001F600", "\xff", "\x80",
	}

	rng := rand.New(rand.NewPCG(1, 2))

	var b strings.Builder
	for b.Len() < length {
		b.WriteString(alphabet[rng.IntN(len(alphabet))])
	}

	return b.String()[:length]
}

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
		// The audit's samples: a pattern that would backtrack, a Unicode class,
		// and a bare quantifier.
		"(a*)*b":    "\\(a\\*\\)\\*b",
		`\p{L}`:     `\\p\{L\}`,
		"{1000000}": "\\{1000000\\}",
	}

	for input, want := range cases {
		if got := Escape(input); got != want {
			t.Errorf("Escape(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestEscapeMatchesTheShippedRuneForm pins that the rewrite changed the cost and
// nothing else: every ASCII rune on its own and in every pair, the inputs that
// are not valid UTF-8, and one 1.1 MB mixed string all come out byte for byte
// the same as the accumulator they replaced.
func TestEscapeMatchesTheShippedRuneForm(t *testing.T) {
	mismatches := 0

	for first := 1; first <= 0x7F; first++ {
		if got, want := Escape(string(rune(first))), escapeRunes(string(rune(first))); got != want {
			mismatches++
			t.Errorf("Escape(%q) = %q, want %q", string(rune(first)), got, want)
		}

		for second := 1; second <= 0x7F; second++ {
			input := string(rune(first)) + string(rune(second))
			if got, want := Escape(input), escapeRunes(input); got != want {
				mismatches++
				t.Errorf("Escape(%q) = %q, want %q", input, got, want)
			}
		}
	}

	wide := []string{
		"",
		"\x00",
		"\xff",
		"\x80abc",
		"осада\xff",
		"(a*)*b",
		`\p{L}`,
		"{1000000}",
		mixedInput(1_100_000),
	}

	for _, input := range wide {
		got, want := Escape(input), escapeRunes(input)
		if got != want {
			mismatches++
			t.Errorf("Escape on a %d byte input differs from the shipped form at byte %d",
				len(input), firstDifference(got, want))
		}
	}

	if mismatches != 0 {
		t.Fatalf("%d mismatches against the shipped form, want 0", mismatches)
	}
}

// firstDifference is the index of the first byte where two strings part, or -1
// when they are equal.
func firstDifference(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}

	if len(a) != len(b) {
		return min(len(a), len(b))
	}

	return -1
}

// TestEscapeAllocatesOncePerCall is the regression guard on the allocation
// count: the Builder sized from the output bound allocates one buffer, while the
// accumulator it replaced measured 23 allocations on this same input.
func TestEscapeAllocatesOncePerCall(t *testing.T) {
	input := strings.Repeat("осада b.*()[]{}", 2048)

	allocs := testing.AllocsPerRun(20, func() {
		sink = Escape(input)
	})

	if allocs > 1 {
		t.Errorf("Escape on a %d byte input allocated %.0f times, want at most 1", len(input), allocs)
	}

	if baseline := testing.AllocsPerRun(20, func() {
		sink = escapeRunes(input)
	}); baseline <= allocs {
		t.Errorf("the shipped accumulator measured %.0f allocations against %.0f: the guard no longer separates the two forms", baseline, allocs)
	}
}

// TestEscapeStaysWithinItsByteBudget is the regression guard on the expansion
// cost. Measured on this checkout, the Builder form spends 2.00 bytes of heap
// per input byte at 1 MiB and at 4 MiB; the accumulator it replaced spent 41.43
// on an ASCII-heavy 1 MiB pattern and 26.74 on a mixed one. The bound sits just
// above the measured figure so a revert fails and a size-class rounding does not.
func TestEscapeStaysWithinItsByteBudget(t *testing.T) {
	const (
		size       = 1 << 20
		runs       = 8
		maxPerByte = 2.5
	)

	input := mixedInput(size)

	for range 3 {
		sink = Escape(input)
	}

	runtime.GC()

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	for range runs {
		sink = Escape(input)
	}

	runtime.ReadMemStats(&after)

	perByte := float64(after.TotalAlloc-before.TotalAlloc) / float64(runs*size)
	if perByte > maxPerByte {
		t.Errorf("Escape spent %.2f bytes of heap per input byte over %d runs of %d bytes, want at most %.2f",
			perByte, runs, size, maxPerByte)
	}

	// The budget only means something while the form it replaced is clearly
	// over it on the same input.
	runtime.GC()
	runtime.ReadMemStats(&before)

	for range runs {
		sink = escapeRunes(input)
	}

	runtime.ReadMemStats(&after)

	if baseline := float64(after.TotalAlloc-before.TotalAlloc) / float64(runs*size); baseline <= maxPerByte {
		t.Errorf("the shipped accumulator measured %.2f bytes per input byte on this input: the budget no longer separates the two forms", baseline)
	}
}
