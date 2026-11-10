package model

import (
	"errors"
	"testing"
	"time"
)

var postNow = time.Date(2026, 10, 7, 18, 0, 0, 0, time.UTC)

func validDetails() Details {
	return Details{
		Kind:       KindSession,
		Activities: []Activity{ActivityMining},
		Title:      "  Идём в шахту  ",
		StartsAt:   new(postNow.Add(20 * time.Minute)),
		Slots:      2,
	}
}

func newTestPost(t *testing.T) *Post {
	t.Helper()
	p, err := NewPost("author", validDetails(), postNow)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNewPostValidates(t *testing.T) {
	cases := map[string]struct {
		mutate func(*Details)
		want   error
	}{
		"unknown kind":      {func(d *Details) { d.Kind = "raid" }, ErrKindInvalid},
		"unknown activity":  {func(d *Details) { d.Activities = []Activity{"fishing"} }, ErrActivityInvalid},
		"no activity":       {func(d *Details) { d.Activities = nil }, ErrActivityInvalid},
		"repeated activity": {func(d *Details) { d.Activities = []Activity{ActivityMining, ActivityMining} }, ErrActivityInvalid},
		"too many activities": {func(d *Details) {
			d.Activities = []Activity{ActivityMining, ActivityBuilding, ActivityFarming, ActivityCombat, ActivityOther}
		}, ErrActivityInvalid},
		"no start":           {func(d *Details) { d.StartsAt = nil }, ErrStartInvalid},
		"short contact":      {func(d *Details) { d.Contact = "ab" }, ErrContactInvalid},
		"short title":        {func(d *Details) { d.Title = " ab " }, ErrTitleInvalid},
		"long title":         {func(d *Details) { d.Title = string(make([]rune, TitleMaxLen+1)) }, ErrTitleInvalid},
		"long description":   {func(d *Details) { d.Description = string(make([]rune, DescriptionMaxLen+1)) }, ErrDescriptionTooLong},
		"zero slots":         {func(d *Details) { d.Slots = 0 }, ErrSlotsInvalid},
		"too many slots":     {func(d *Details) { d.Slots = SlotsMax + 1 }, ErrSlotsInvalid},
		"starts long ago":    {func(d *Details) { d.StartsAt = new(postNow.Add(-MaxLag - time.Minute)) }, ErrStartInvalid},
		"starts too far out": {func(d *Details) { d.StartsAt = new(postNow.Add(MaxLead + time.Minute)) }, ErrStartInvalid},
	}
	for name, c := range cases {
		d := validDetails()
		c.mutate(&d)
		if _, err := NewPost("author", d, postNow); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", name, c.want, err)
		}
	}

	if _, err := NewPost(" ", validDetails(), postNow); !errors.Is(err, ErrAuthorRequired) {
		t.Errorf("blank author: want ErrAuthorRequired, got %v", err)
	}
}

func TestNewPostTrimsAndOpens(t *testing.T) {
	p := newTestPost(t)
	if p.Title != "Идём в шахту" {
		t.Fatalf("title not trimmed: %q", p.Title)
	}
	if !p.IsOpen(postNow) || p.Responders == nil {
		t.Fatal("new post should be open with an empty responder list")
	}
	if !p.ExpiresAt.Equal(p.StartsAt.Add(SessionLifetime)) {
		t.Fatalf("expires_at %v", p.ExpiresAt)
	}
	if len(p.PlayDays) != 0 || p.Contact != "" {
		t.Fatal("a session keeps no schedule")
	}
}

func TestRespondTogglesAndFills(t *testing.T) {
	p := newTestPost(t)

	if joined, _, err := p.Respond("a", postNow); err != nil || !joined {
		t.Fatalf("a joins: %v %v", joined, err)
	}
	if joined, _, err := p.Respond("b", postNow); err != nil || !joined {
		t.Fatalf("b joins: %v %v", joined, err)
	}
	if _, _, err := p.Respond("c", postNow); !errors.Is(err, ErrPostFull) {
		t.Fatalf("c on a full post: want ErrPostFull, got %v", err)
	}
	if joined, _, err := p.Respond("a", postNow); err != nil || joined {
		t.Fatalf("a leaves: %v %v", joined, err)
	}
	if joined, _, err := p.Respond("c", postNow); err != nil || !joined {
		t.Fatalf("c takes the freed slot: %v %v", joined, err)
	}
	if got := p.Responders; len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("responders out of join order: %v", got)
	}
}

func TestRespondRules(t *testing.T) {
	p := newTestPost(t)
	if _, _, err := p.Respond("author", postNow); !errors.Is(err, ErrOwnPost) {
		t.Fatalf("own post: want ErrOwnPost, got %v", err)
	}
	if _, _, err := p.Respond("a", p.ExpiresAt); !errors.Is(err, ErrPostClosed) {
		t.Fatalf("after expiry: want ErrPostClosed, got %v", err)
	}

	_ = p.Close("author", postNow)
	if _, _, err := p.Respond("a", postNow); !errors.Is(err, ErrPostClosed) {
		t.Fatalf("closed: want ErrPostClosed, got %v", err)
	}
}

// TestRespondNotifiesOncePerResponder is the regression for the unbounded
// notification write. Respond is a toggle and both directions are free, so
// gating the author notification on joined let one responder produce one
// notification per withdraw/rejoin cycle for as long as the post stayed up —
// 1000 documents from 2000 calls, none of them removable by the victim.
//
// joined stays true on every rejoin: it answers "are you in now" per the proto
// contract, and only notify is deduped.
func TestRespondNotifiesOncePerResponder(t *testing.T) {
	p := newTestPost(t)

	notifies := 0
	joins := 0
	for cycle := range 5 {
		at := postNow.Add(time.Duration(cycle) * time.Minute)

		joined, notify, err := p.Respond("a", at)
		if err != nil || !joined {
			t.Fatalf("cycle %d join: joined=%v err=%v", cycle, joined, err)
		}
		joins += b2i(joined)
		notifies += b2i(notify)

		joined, notify, err = p.Respond("a", at)
		if err != nil || joined || notify {
			t.Fatalf("cycle %d withdraw: joined=%v notify=%v err=%v", cycle, joined, notify, err)
		}
		notifies += b2i(notify)
	}

	// The fifth cycle ends on a withdraw, so join once more: the sixth join of an
	// already-notified responder must keep joined=true and notify=false.
	joined, notify, err := p.Respond("a", postNow.Add(5*time.Minute))
	if err != nil || !joined || notify {
		t.Fatalf("rejoin after five cycles: joined=%v notify=%v err=%v", joined, notify, err)
	}
	joins += b2i(joined)
	notifies += b2i(notify)

	if joins != 6 {
		t.Fatalf("a joined %d times, want 6: joined must stay true on every rejoin", joins)
	}
	if notifies != 1 {
		t.Fatalf("a notified the author %d times, want exactly 1", notifies)
	}
	if !p.HasResponder("a") {
		t.Fatalf("a is in the post but not a responder: %v", p.Responders)
	}
	if got := p.Notified; len(got) != 1 || got[0] != "a" {
		t.Fatalf("Notified must hold a once: %v", got)
	}
}

// A second responder is a second notification: the dedupe is per responder, not
// per post, or the author would never hear about anyone after the first.
func TestRespondNotifiesEachResponderOnce(t *testing.T) {
	p := newTestPost(t)
	p.Slots = 3

	notifies := 0
	for _, id := range []string{"a", "b", "c"} {
		if _, notify, err := p.Respond(id, postNow); err != nil || !notify {
			t.Fatalf("%s first join: notify=%v err=%v", id, notify, err)
		}
		notifies++
	}
	if notifies != 3 {
		t.Fatalf("first joins notified %d times, want 3", notifies)
	}

	for _, id := range []string{"a", "b", "c"} {
		if _, _, err := p.Respond(id, postNow); err != nil {
			t.Fatalf("%s withdraw: %v", id, err)
		}
		if joined, notify, err := p.Respond(id, postNow); err != nil || !joined || notify {
			t.Fatalf("%s rejoin: joined=%v notify=%v err=%v", id, joined, notify, err)
		}
	}
	if got := p.Notified; len(got) != 3 {
		t.Fatalf("Notified grew past its responders: %v", got)
	}
}

// Posts stored before Notified existed decode it as empty while Responders
// survives, so one already-seen responder notifies once more after a
// withdraw/rejoin — and then never again. That one-shot cost is the migration
// boundary; without this test it is invisible.
func TestRespondLegacyPostNotifiesOnceMore(t *testing.T) {
	p := newTestPost(t)
	p.Responders = []string{"a"} // a document written before Notified existed
	p.Notified = nil

	if joined, _, err := p.Respond("a", postNow); err != nil || joined {
		t.Fatalf("legacy withdraw: joined=%v err=%v", joined, err)
	}
	if joined, notify, err := p.Respond("a", postNow); err != nil || !joined || !notify {
		t.Fatalf("legacy rejoin: joined=%v notify=%v err=%v", joined, notify, err)
	}
	if _, notify, err := p.Respond("a", postNow); err != nil || notify {
		t.Fatalf("second legacy withdraw: notify=%v err=%v", notify, err)
	}
	if joined, notify, err := p.Respond("a", postNow); err != nil || !joined || notify {
		t.Fatalf("second legacy rejoin: joined=%v notify=%v err=%v", joined, notify, err)
	}
}

// A teammate post marks interest instead of filling slots, and its lifetime is
// 14 days renewable, so it is the longer window for the same toggle.
func TestTeammateRespondNotifiesOnce(t *testing.T) {
	p := newTeammatePost(t)

	if _, notify, err := p.Respond("a", postNow); err != nil || !notify {
		t.Fatalf("first interest: notify=%v err=%v", notify, err)
	}
	for range 3 {
		if _, _, err := p.Respond("a", postNow); err != nil {
			t.Fatal(err)
		}
		if joined, notify, err := p.Respond("a", postNow); err != nil || !joined || notify {
			t.Fatalf("re-interest: joined=%v notify=%v err=%v", joined, notify, err)
		}
	}
	if got := p.Notified; len(got) != 1 || got[0] != "a" {
		t.Fatalf("Notified: %v", got)
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestCloseAuthorOnly(t *testing.T) {
	p := newTestPost(t)
	if err := p.Close("stranger", postNow); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("stranger: want ErrNotAuthor, got %v", err)
	}
	if err := p.Close("author", postNow); err != nil {
		t.Fatal(err)
	}
	_ = p.Close("author", postNow.Add(time.Hour))
	if !p.ClosedAt.Equal(postNow) || p.IsOpen(postNow) {
		t.Fatalf("close moment moved or post still open: %v", p.ClosedAt)
	}
}

func teammateDetails() Details {
	return Details{
		Kind:       KindTeammate,
		Activities: []Activity{ActivityBuilding, ActivityFarming},
		Title:      "Ищу напарника на долгую игру",
		Slots:      1,
		PlayDays:   []int32{6, 1, 7},
		PlayTimes:  []PlayTime{PlayTimeEvening},
		Experience: ExperienceExperienced,
		Voice:      true,
		Contact:    " discord: fox ",
		// A teammate post has no start; a stray one is ignored.
		StartsAt: new(postNow.Add(-30 * 24 * time.Hour)),
	}
}

func newTeammatePost(t *testing.T) *Post {
	t.Helper()
	p, err := NewPost("author", teammateDetails(), postNow)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNewTeammatePost(t *testing.T) {
	p := newTeammatePost(t)

	if p.StartsAt != nil {
		t.Fatalf("teammate post must have no start, got %v", p.StartsAt)
	}
	if !p.ExpiresAt.Equal(postNow.Add(TeammateLifetime)) || !p.BumpedAt.Equal(postNow) {
		t.Fatalf("expires %v bumped %v", p.ExpiresAt, p.BumpedAt)
	}
	if got := p.PlayDays; len(got) != 3 || got[0] != 1 || got[1] != 6 || got[2] != 7 {
		t.Fatalf("play days not sorted: %v", got)
	}
	if p.Contact != "discord: fox" || !p.Voice || p.Experience != ExperienceExperienced {
		t.Fatalf("details lost: %+v", p)
	}
}

func TestNewTeammatePostValidates(t *testing.T) {
	cases := map[string]struct {
		mutate func(*Details)
		want   error
	}{
		"no contact":         {func(d *Details) { d.Contact = "  " }, ErrContactInvalid},
		"long contact":       {func(d *Details) { d.Contact = string(make([]rune, ContactMaxLen+1)) }, ErrContactInvalid},
		"day zero":           {func(d *Details) { d.PlayDays = []int32{0} }, ErrScheduleInvalid},
		"day eight":          {func(d *Details) { d.PlayDays = []int32{8} }, ErrScheduleInvalid},
		"repeated day":       {func(d *Details) { d.PlayDays = []int32{2, 2} }, ErrScheduleInvalid},
		"unknown time":       {func(d *Details) { d.PlayTimes = []PlayTime{"dawn"} }, ErrScheduleInvalid},
		"unknown experience": {func(d *Details) { d.Experience = "pro" }, ErrExperienceInvalid},
	}
	for name, c := range cases {
		d := teammateDetails()
		c.mutate(&d)
		if _, err := NewPost("author", d, postNow); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", name, c.want, err)
		}
	}

	d := teammateDetails()
	d.PlayDays, d.PlayTimes, d.Experience = nil, nil, ExperienceUnset
	if _, err := NewPost("author", d, postNow); err != nil {
		t.Fatalf("schedule and experience are optional: %v", err)
	}
}

func TestTeammateInterestIgnoresSlots(t *testing.T) {
	p := newTeammatePost(t)

	for _, id := range []string{"a", "b", "c"} {
		if in, _, err := p.Respond(id, postNow); err != nil || !in {
			t.Fatalf("%s interested: %v %v", id, in, err)
		}
	}

	p.Responders = make([]string, MaxInterested)
	if _, _, err := p.Respond("late", postNow); !errors.Is(err, ErrPostFull) {
		t.Fatalf("over the cap: want ErrPostFull, got %v", err)
	}
}

func TestRenew(t *testing.T) {
	p := newTeammatePost(t)

	if err := p.Renew("stranger", postNow.Add(48*time.Hour)); !errors.Is(err, ErrNotAuthor) {
		t.Fatalf("stranger: want ErrNotAuthor, got %v", err)
	}
	if err := p.Renew("author", postNow.Add(RenewCooldown-time.Minute)); !errors.Is(err, ErrRenewTooSoon) {
		t.Fatalf("too soon: want ErrRenewTooSoon, got %v", err)
	}

	// An expired post comes back.
	later := postNow.Add(TeammateLifetime + time.Hour)
	if p.IsOpen(later) {
		t.Fatal("post should have expired")
	}
	if err := p.Renew("author", later); err != nil {
		t.Fatal(err)
	}
	if !p.IsOpen(later) || !p.BumpedAt.Equal(later) || !p.ExpiresAt.Equal(later.Add(TeammateLifetime)) {
		t.Fatalf("renew did not reopen: %+v", p)
	}

	_ = p.Close("author", later)
	if err := p.Renew("author", later.Add(48*time.Hour)); !errors.Is(err, ErrPostClosed) {
		t.Fatalf("closed: want ErrPostClosed, got %v", err)
	}

	s := newTestPost(t)
	if err := s.Renew("author", postNow.Add(48*time.Hour)); !errors.Is(err, ErrNotRenewable) {
		t.Fatalf("session: want ErrNotRenewable, got %v", err)
	}
}
