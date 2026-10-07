package model

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// TitleMinLen and TitleMaxLen bound the title, in characters.
	TitleMinLen = 3
	TitleMaxLen = 80
	// DescriptionMaxLen bounds the description, in characters.
	DescriptionMaxLen = 500
	// ContactMinLen and ContactMaxLen bound the contact, in characters.
	ContactMinLen = 3
	ContactMaxLen = 64
	// SlotsMax is the most companions one post may look for.
	SlotsMax = 10
	// ActivitiesMax is how many activities one post may list.
	ActivitiesMax = 4

	// MaxOpenSessionsPerAuthor is how many open session posts one player may keep.
	MaxOpenSessionsPerAuthor = 3
	// MaxOpenTeammatePerAuthor is how many open teammate posts one player may keep.
	MaxOpenTeammatePerAuthor = 1

	// SessionLifetime is how long after its start a session post stays on the board.
	SessionLifetime = 4 * time.Hour
	// MaxLead is how far ahead a session may be planned.
	MaxLead = 7 * 24 * time.Hour
	// MaxLag is how far in the past starts_at may be ("we started a moment ago").
	MaxLag = time.Hour

	// TeammateLifetime is how long a teammate post stays up after it is posted
	// or renewed.
	TeammateLifetime = 14 * 24 * time.Hour
	// RenewCooldown is how often a teammate post may be renewed (and so lifted
	// to the top of the board).
	RenewCooldown = 24 * time.Hour
	// MaxInterested caps the "interested" list of a teammate post.
	MaxInterested = 50
)

// Errors returned by the model. The service maps them to typed domain errors.
var (
	ErrKindInvalid        = errors.New("unknown post kind")
	ErrActivityInvalid    = errors.New("activities must be 1 to 4 distinct known activities")
	ErrTitleInvalid       = errors.New("title must be 3 to 80 characters")
	ErrDescriptionTooLong = errors.New("description is too long")
	ErrSlotsInvalid       = errors.New("slots must be between 1 and 10")
	ErrStartInvalid       = errors.New("starts_at must be within an hour ago and 7 days ahead")
	ErrScheduleInvalid    = errors.New("play days must be distinct days 1..7 and play times distinct known times")
	ErrExperienceInvalid  = errors.New("unknown experience")
	ErrContactInvalid     = errors.New("contact must be 3 to 64 characters, required for a teammate post")
	ErrAuthorRequired     = errors.New("author is required")
	ErrPostClosed         = errors.New("post is closed or over")
	ErrOwnPost            = errors.New("cannot respond to your own post")
	ErrPostFull           = errors.New("all slots are taken")
	ErrNotAuthor          = errors.New("only the author can change the post")
	ErrNotRenewable       = errors.New("only a teammate post can be renewed")
	ErrRenewTooSoon       = errors.New("the post was renewed less than a day ago")
)

// Kind tells a one-off outing from a search for a regular teammate.
type Kind string

const (
	// KindSession is "going to the mines in 20 minutes, need two more".
	KindSession Kind = "session"
	// KindTeammate is "looking for someone to play with regularly".
	KindTeammate Kind = "teammate"
)

// IsValid reports whether k is a known kind.
func (k Kind) IsValid() bool {
	return k == KindSession || k == KindTeammate
}

// Activity is what the company is going to do.
type Activity string

const (
	ActivityMining    Activity = "mining"
	ActivityExploring Activity = "exploring"
	ActivityBuilding  Activity = "building"
	ActivityHunting   Activity = "hunting"
	ActivityTrading   Activity = "trading"
	ActivityFarming   Activity = "farming"
	ActivityCombat    Activity = "combat"
	ActivityOther     Activity = "other"
)

// IsValid reports whether a is a known activity.
func (a Activity) IsValid() bool {
	switch a {
	case ActivityMining, ActivityExploring, ActivityBuilding, ActivityHunting,
		ActivityTrading, ActivityFarming, ActivityCombat, ActivityOther:
		return true
	default:
		return false
	}
}

// PlayTime is a part of the day a teammate usually plays.
type PlayTime string

const (
	PlayTimeMorning PlayTime = "morning"
	PlayTimeDay     PlayTime = "day"
	PlayTimeEvening PlayTime = "evening"
	PlayTimeNight   PlayTime = "night"
)

// IsValid reports whether t is a known part of the day.
func (t PlayTime) IsValid() bool {
	switch t {
	case PlayTimeMorning, PlayTimeDay, PlayTimeEvening, PlayTimeNight:
		return true
	default:
		return false
	}
}

// Experience is how long the author has played Vintage Story.
type Experience string

const (
	// ExperienceUnset means the author did not say.
	ExperienceUnset       Experience = ""
	ExperienceNewbie      Experience = "newbie"
	ExperienceExperienced Experience = "experienced"
	ExperienceVeteran     Experience = "veteran"
)

// IsValid reports whether e is a known experience or unset.
func (e Experience) IsValid() bool {
	switch e {
	case ExperienceUnset, ExperienceNewbie, ExperienceExperienced, ExperienceVeteran:
		return true
	default:
		return false
	}
}

// Details are the fields an author sets when posting.
type Details struct {
	Kind        Kind
	Activities  []Activity
	Title       string
	Description string
	// StartsAt is required for a session and ignored for a teammate post.
	StartsAt *time.Time
	Slots    int32
	// PlayDays (1 = Monday … 7 = Sunday), PlayTimes, Experience and Voice
	// describe a teammate; a session ignores them.
	PlayDays   []int32
	PlayTimes  []PlayTime
	Experience Experience
	Voice      bool
	// Contact is how to reach the author (Discord, Telegram…). Required for a
	// teammate post, optional for a session.
	Contact string
}

// Post is a "looking for company" announcement.
type Post struct {
	Id          string
	AuthorId    string
	Kind        Kind
	Activities  []Activity
	Title       string
	Description string
	// StartsAt is set for a session only.
	StartsAt *time.Time
	// Slots is how many companions are wanted, the author not counted.
	Slots int32
	// Responders are the players who joined a session, or who are interested in
	// a teammate post, in the order they responded.
	Responders []string
	PlayDays   []int32
	PlayTimes  []PlayTime
	Experience Experience
	Voice      bool
	Contact    string
	ClosedAt   *time.Time
	// ExpiresAt is when the post leaves the board.
	ExpiresAt time.Time
	// BumpedAt is when the post was posted or last renewed; teammate posts are
	// listed by it, freshest first.
	BumpedAt  time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewPost validates details and builds an open post.
func NewPost(authorId string, d Details, now time.Time) (*Post, error) {
	if strings.TrimSpace(authorId) == "" {
		return nil, ErrAuthorRequired
	}
	if !d.Kind.IsValid() {
		return nil, ErrKindInvalid
	}

	title := strings.TrimSpace(d.Title)
	description := strings.TrimSpace(d.Description)
	contact := strings.TrimSpace(d.Contact)

	switch {
	case !validActivities(d.Activities):
		return nil, ErrActivityInvalid
	case utf8.RuneCountInString(title) < TitleMinLen, utf8.RuneCountInString(title) > TitleMaxLen:
		return nil, ErrTitleInvalid
	case utf8.RuneCountInString(description) > DescriptionMaxLen:
		return nil, ErrDescriptionTooLong
	case d.Slots < 1, d.Slots > SlotsMax:
		return nil, ErrSlotsInvalid
	case !validContact(contact, d.Kind == KindTeammate):
		return nil, ErrContactInvalid
	}

	post := &Post{
		AuthorId:    authorId,
		Kind:        d.Kind,
		Activities:  slices.Clone(d.Activities),
		Title:       title,
		Description: description,
		Slots:       d.Slots,
		Responders:  []string{},
		Contact:     contact,
		BumpedAt:    now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if d.Kind == KindSession {
		if d.StartsAt == nil || d.StartsAt.Before(now.Add(-MaxLag)) || d.StartsAt.After(now.Add(MaxLead)) {
			return nil, ErrStartInvalid
		}
		starts := d.StartsAt.UTC()
		post.StartsAt = &starts
		post.ExpiresAt = starts.Add(SessionLifetime)
		post.PlayDays = []int32{}
		post.PlayTimes = []PlayTime{}
		return post, nil
	}

	if !validDays(d.PlayDays) || !validTimes(d.PlayTimes) {
		return nil, ErrScheduleInvalid
	}
	if !d.Experience.IsValid() {
		return nil, ErrExperienceInvalid
	}
	post.PlayDays = sortedDays(d.PlayDays)
	post.PlayTimes = append([]PlayTime{}, d.PlayTimes...)
	post.Experience = d.Experience
	post.Voice = d.Voice
	post.ExpiresAt = now.Add(TeammateLifetime)
	return post, nil
}

// IsOpen reports whether the post is still on the board at now.
func (p *Post) IsOpen(now time.Time) bool {
	return p.ClosedAt == nil && now.Before(p.ExpiresAt)
}

// IsFull reports whether nobody else can respond: every session slot is
// taken, or the teammate post reached MaxInterested.
func (p *Post) IsFull() bool {
	limit := p.Slots
	if p.Kind == KindTeammate {
		limit = MaxInterested
	}
	return int32(len(p.Responders)) >= limit
}

// HasResponder reports whether userId responded to the post.
func (p *Post) HasResponder(userId string) bool {
	return slices.Contains(p.Responders, userId)
}

// Respond toggles userId's response: joins the session (or marks interest in a
// teammate post) if they have not responded, withdraws otherwise. Reports
// whether they are in afterwards.
//
// Withdrawing is allowed only while the post is open, like responding: a
// closed post is history and stays as it was.
func (p *Post) Respond(userId string, now time.Time) (bool, error) {
	if !p.IsOpen(now) {
		return false, ErrPostClosed
	}
	if userId == p.AuthorId {
		return false, ErrOwnPost
	}

	if p.HasResponder(userId) {
		p.Responders = slices.DeleteFunc(p.Responders, func(id string) bool { return id == userId })
		return false, nil
	}

	if p.IsFull() {
		return false, ErrPostFull
	}
	p.Responders = append(p.Responders, userId)
	return true, nil
}

// Close takes the post off the board. Author only; closing twice keeps the
// first moment.
func (p *Post) Close(userId string, now time.Time) error {
	if userId != p.AuthorId {
		return ErrNotAuthor
	}
	if p.ClosedAt == nil {
		p.ClosedAt = &now
	}
	return nil
}

// Renew keeps a teammate post up for another TeammateLifetime and lifts it to
// the top of the board. Author only, at most once per RenewCooldown. An
// expired post can be renewed while it is still stored; a closed one cannot.
func (p *Post) Renew(userId string, now time.Time) error {
	switch {
	case userId != p.AuthorId:
		return ErrNotAuthor
	case p.Kind != KindTeammate:
		return ErrNotRenewable
	case p.ClosedAt != nil:
		return ErrPostClosed
	case now.Before(p.RenewableAt()):
		return ErrRenewTooSoon
	}

	p.BumpedAt = now
	p.ExpiresAt = now.Add(TeammateLifetime)
	return nil
}

// RenewableAt is the earliest moment the post can be renewed again.
func (p *Post) RenewableAt() time.Time {
	return p.BumpedAt.Add(RenewCooldown)
}

// Touch stamps the persisted update time (called by mongox.UpdateDoc).
func (p *Post) Touch(now time.Time) { p.UpdatedAt = now }

func validActivities(activities []Activity) bool {
	if len(activities) < 1 || len(activities) > ActivitiesMax {
		return false
	}
	for i, a := range activities {
		if !a.IsValid() || slices.Contains(activities[:i], a) {
			return false
		}
	}
	return true
}

func validDays(days []int32) bool {
	for i, d := range days {
		if d < 1 || d > 7 || slices.Contains(days[:i], d) {
			return false
		}
	}
	return true
}

func validTimes(times []PlayTime) bool {
	for i, t := range times {
		if !t.IsValid() || slices.Contains(times[:i], t) {
			return false
		}
	}
	return true
}

func validContact(contact string, required bool) bool {
	n := utf8.RuneCountInString(contact)
	if n == 0 {
		return !required
	}
	return n >= ContactMinLen && n <= ContactMaxLen
}

func sortedDays(days []int32) []int32 {
	out := slices.Clone(days)
	slices.Sort(out)
	if out == nil {
		out = []int32{}
	}
	return out
}
