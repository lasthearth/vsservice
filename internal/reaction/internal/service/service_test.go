package service

import (
	"slices"
	"testing"

	"github.com/lasthearth/vsservice/internal/reaction/internal/model"
)

func TestGroupCountsKeepsRequestAndEmojiOrder(t *testing.T) {
	got := groupCounts(
		[]string{"news:b", "news:a", "event:c"},
		[]model.Count{
			{Target: "news:a", Emoji: "fire", Count: 2},
			{Target: "news:a", Emoji: "like", Count: 5},
			{Target: "news:b", Emoji: "swords", Count: 1},
			{Target: "news:a", Emoji: "heart", Count: 0},
		},
	)

	if len(got) != 3 {
		t.Fatalf("want 3 targets, got %d", len(got))
	}
	if got[0].GetTarget() != "news:b" || got[1].GetTarget() != "news:a" || got[2].GetTarget() != "event:c" {
		t.Fatalf("targets out of request order: %v", got)
	}

	var emojis []string
	for _, c := range got[1].GetCounts() {
		emojis = append(emojis, c.GetEmoji())
	}
	if !slices.Equal(emojis, []string{"like", "fire"}) {
		t.Errorf("want [like fire] (fixed order, zero dropped), got %v", emojis)
	}
	if len(got[2].GetCounts()) != 0 {
		t.Errorf("target without reactions must have no counts, got %v", got[2].GetCounts())
	}
}

func TestOrderedFollowsEmojiList(t *testing.T) {
	got := ordered([]string{"swords", "like", "unknown"})
	if !slices.Equal(got, []string{"like", "swords"}) {
		t.Errorf("got %v", got)
	}
}
