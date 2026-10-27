package model

import (
	"strings"
	"testing"
)

func TestSlugifyTransliteratesRussian(t *testing.T) {
	cases := map[string]string{
		"Как объявить набег": "kak-obyavit-nabeg",
		"Поселения":          "poseleniya",
		"Ёлки и ёжики":       "elki-i-ezhiki",
		"Сводка за 5 дней!":  "svodka-za-5-dney",
		"":                   "",
		"!!!":                "",
	}

	for input, want := range cases {
		if got := Slugify(input); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSlugifyTruncatesWithoutTrailingDash(t *testing.T) {
	long := strings.Repeat("poselenie ", 30)

	got := Slugify(long)
	if len(got) > MaxSlugLength {
		t.Fatalf("slug length = %d, want at most %d", len(got), MaxSlugLength)
	}
	if len(got) > 0 && got[len(got)-1] == '-' {
		t.Fatalf("slug %q ends with a dash", got)
	}
}

func TestIsValidSlug(t *testing.T) {
	valid := []string{"start", "kak-nachat", "rule-5-2"}
	for _, slug := range valid {
		if !IsValidSlug(slug) {
			t.Errorf("IsValidSlug(%q) = false, want true", slug)
		}
	}

	invalid := []string{"", "Начало", "-start", "start-", "start--end", "start end", "Start"}
	for _, slug := range invalid {
		if IsValidSlug(slug) {
			t.Errorf("IsValidSlug(%q) = true, want false", slug)
		}
	}
}
