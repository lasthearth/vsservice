package model

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

// referralCodeCharset is the set of characters used for generated referral
// codes. Uppercase letters and digits only, to keep codes easy to read and
// share between players.
const referralCodeCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// referralCodeLength is the number of characters in a generated referral code.
const referralCodeLength = 8

// ReferralCode is a unique code a player can share to refer other players.
type ReferralCode struct {
	Id         string
	PlayerID   string // Logto user ID of the code's owner.
	PlayerName string // Display name of the code's owner.
	Code       string
	CreatedAt  time.Time
}

// GenerateCode creates a new ReferralCode for playerID/playerName with a
// random 8-character uppercase alphanumeric code. The code is generated with
// crypto/rand since it is shared with other players and must not be guessable.
func GenerateCode(playerID, playerName string) *ReferralCode {
	return &ReferralCode{
		PlayerID:   playerID,
		PlayerName: playerName,
		Code:       newRandomCode(),
		CreatedAt:  time.Now(),
	}
}

// ReconstituteReferralCode rebuilds a ReferralCode from persisted state. Repository use only.
func ReconstituteReferralCode(id, playerID, playerName, code string, createdAt time.Time) *ReferralCode {
	return &ReferralCode{
		Id:         id,
		PlayerID:   playerID,
		PlayerName: playerName,
		Code:       code,
		CreatedAt:  createdAt,
	}
}

// newRandomCode generates a random referralCodeLength-character code drawn
// from referralCodeCharset using crypto/rand.
func newRandomCode() string {
	buf := make([]byte, referralCodeLength)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand.Read on supported platforms only fails if the system
		// entropy source is unavailable, which is unrecoverable.
		panic("model: failed to read random bytes for referral code: " + err.Error())
	}

	code := make([]byte, referralCodeLength)
	for i, b := range buf {
		code[i] = referralCodeCharset[int(b)%len(referralCodeCharset)]
	}
	return string(code)
}

// verificationSharePercent is the part of the referrer's reward paid when the
// invited player passes verification. The rest is paid on that player's first
// join, and JoinShare takes it as the remainder rather than a second
// percentage, so the two shares always add up to the full reward and no coin is
// lost to rounding.
const verificationSharePercent = 25

// checkBackoff is how long the payout loop waits before looking at an event
// again after a pass found nothing to pay for it. A referral whose player never
// verifies or never joins stays owed forever, and those events would otherwise
// fill every pass and hide the referral that just moved. The staircase keeps a
// fresh referral quick (the first two steps are inside the minute-scale window
// where a friend usually shows up) and turns a dead one into one check an hour.
var checkBackoff = []time.Duration{
	time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	time.Hour,
}

// RewardStage names one part of a referral reward. The stages are reached at
// different moments, so each is claimed and paid on its own: a stage that fails
// is retried by itself, and a stage already paid is never paid twice.
type RewardStage string

const (
	// StageVerification is the referrer's share paid when the referee passes
	// verification on the site.
	StageVerification RewardStage = "verification"
	// StageJoin is the referrer's remaining share, paid when the referee first
	// joins the game server.
	StageJoin RewardStage = "join"
	// StageReferee is the referee's own reward, paid on that same first join.
	StageReferee RewardStage = "referee"
)

// Reward is one payable part of a referral event: which stage pays, who is
// credited, and how much.
type Reward struct {
	Stage RewardStage
	// Payee is the player id that Coins is credited to.
	Payee string
	Coins int64
}

// ErrStageAlreadyPaid and ErrStageNotPaid report a stage transition that the
// event is not in a state for. They are programming errors at a call site, not
// player-facing outcomes: the payout loop asks PendingRewards before it pays.
var (
	ErrStageAlreadyPaid = errors.New("referral reward stage already paid")
	ErrStageNotPaid     = errors.New("referral reward stage not paid")
	ErrUnknownStage     = errors.New("unknown referral reward stage")
)

// ReferralEvent records that refereePlayerID was referred by referrerPlayerID,
// what the two sides are owed, and which parts have been paid.
//
// Nothing is paid when the code is applied. The referrer is paid in two parts,
// on the referee's verification and on the referee's first join; the referee is
// paid once, on that same first join. CoinsAwarded counts what the referrer has
// actually received, so the stats endpoint reports the same number the wallet
// holds instead of a promise.
type ReferralEvent struct {
	Id               string
	ReferrerPlayerID string
	RefereePlayerID  string
	// ReferrerReward and RefereeReward are the full rewards, taken from the
	// config when the code was applied. Storing them keeps a later config change
	// from repricing referrals that are already in flight.
	ReferrerReward int64
	RefereeReward  int64
	// CoinsAwarded is how many coins the referrer has received so far.
	CoinsAwarded     int64
	VerificationPaid bool
	JoinPaid         bool
	RefereePaid      bool
	// Checks counts how many passes looked at this event and found nothing to
	// pay, and NextCheckAt is when the next one may look again. Both exist so the
	// loop's working set stays small; see DeferNext.
	Checks      int
	NextCheckAt time.Time
	CreatedAt   time.Time
}

// NewReferralEvent creates a new ReferralEvent recording that refereePlayerID
// was referred by referrerPlayerID, owing referrerReward to the referrer and
// refereeReward to the referee. Nothing is paid yet.
func NewReferralEvent(referrerPlayerID, refereePlayerID string, referrerReward, refereeReward int64) *ReferralEvent {
	now := time.Now()

	return &ReferralEvent{
		ReferrerPlayerID: referrerPlayerID,
		RefereePlayerID:  refereePlayerID,
		ReferrerReward:   referrerReward,
		RefereeReward:    refereeReward,
		// Due at once: a player can verify or join before the first pass sees the
		// event at all.
		NextCheckAt: now,
		CreatedAt:   now,
	}
}

// ReconstituteReferralEvent rebuilds a ReferralEvent from persisted state. Repository use only.
func ReconstituteReferralEvent(
	id, referrerPlayerID, refereePlayerID string,
	referrerReward, refereeReward, coinsAwarded int64,
	verificationPaid, joinPaid, refereePaid bool,
	checks int,
	nextCheckAt, createdAt time.Time,
) *ReferralEvent {
	return &ReferralEvent{
		Id:               id,
		ReferrerPlayerID: referrerPlayerID,
		RefereePlayerID:  refereePlayerID,
		ReferrerReward:   referrerReward,
		RefereeReward:    refereeReward,
		CoinsAwarded:     coinsAwarded,
		VerificationPaid: verificationPaid,
		JoinPaid:         joinPaid,
		RefereePaid:      refereePaid,
		Checks:           checks,
		NextCheckAt:      nextCheckAt,
		CreatedAt:        createdAt,
	}
}

// VerificationShare is the part of the referrer's reward paid on verification.
func (e *ReferralEvent) VerificationShare() int64 {
	if e.ReferrerReward <= 0 {
		return 0
	}
	return e.ReferrerReward * verificationSharePercent / 100
}

// JoinShare is the rest of the referrer's reward, paid on the first join.
func (e *ReferralEvent) JoinShare() int64 {
	if e.ReferrerReward <= 0 {
		return 0
	}
	return e.ReferrerReward - e.VerificationShare()
}

// Pays reports whether stage pays anything at all. A reward configured as zero
// is a reward turned off, not a stage to claim and then skip.
func (e *ReferralEvent) Pays(stage RewardStage) bool {
	return e.Coins(stage) > 0
}

// Coins is the amount stage pays, or 0 for a stage that pays nothing or is not
// one of the three this event knows.
func (e *ReferralEvent) Coins(stage RewardStage) int64 {
	switch stage {
	case StageVerification:
		return e.VerificationShare()
	case StageJoin:
		return e.JoinShare()
	case StageReferee:
		if e.RefereeReward <= 0 {
			return 0
		}
		return e.RefereeReward
	default:
		return 0
	}
}

// Payee is the player id stage credits: the referee for their own reward, the
// referrer for both of theirs.
func (e *ReferralEvent) Payee(stage RewardStage) string {
	switch stage {
	case StageVerification, StageJoin:
		return e.ReferrerPlayerID
	case StageReferee:
		return e.RefereePlayerID
	default:
		return ""
	}
}

// Paid reports whether stage has already been claimed.
func (e *ReferralEvent) Paid(stage RewardStage) bool {
	switch stage {
	case StageVerification:
		return e.VerificationPaid
	case StageJoin:
		return e.JoinPaid
	case StageReferee:
		return e.RefereePaid
	default:
		return false
	}
}

// PendingRewards lists what this event still owes, in the order it must be paid.
// verified and joined say how far the referee has got, read from their player
// document.
//
// A join also pays the verification share, because the game server only writes
// presence for a player it has let in: a referee in the game has passed
// verification by then, even if the payout loop never saw the approval.
func (e *ReferralEvent) PendingRewards(verified, joined bool) []Reward {
	rewards := make([]Reward, 0, 3)

	appendStage := func(stage RewardStage) {
		if e.Paid(stage) || !e.Pays(stage) {
			return
		}
		rewards = append(rewards, Reward{Stage: stage, Payee: e.Payee(stage), Coins: e.Coins(stage)})
	}

	if verified || joined {
		appendStage(StageVerification)
	}
	if joined {
		appendStage(StageJoin)
		appendStage(StageReferee)
	}

	return rewards
}

// MarkPaid records that stage was paid and, for the referrer's stages, adds its
// coins to the running total. It refuses a stage that is already paid, so a
// caller cannot bill the same share twice.
func (e *ReferralEvent) MarkPaid(stage RewardStage) error {
	if !e.Pays(stage) {
		return fmt.Errorf("%w: %q", ErrUnknownStage, stage)
	}
	if e.Paid(stage) {
		return fmt.Errorf("%w: %q", ErrStageAlreadyPaid, stage)
	}

	switch stage {
	case StageVerification:
		e.VerificationPaid = true
	case StageJoin:
		e.JoinPaid = true
	case StageReferee:
		e.RefereePaid = true
	}

	if stage != StageReferee {
		e.CoinsAwarded += e.Coins(stage)
	}

	return nil
}

// UnmarkPaid reverses MarkPaid when the payout of a claimed stage failed, so the
// next pass can try it again. It refuses a stage that was never paid, because
// undoing that would credit the running total with coins nobody owed.
func (e *ReferralEvent) UnmarkPaid(stage RewardStage) error {
	if !e.Paid(stage) {
		return fmt.Errorf("%w: %q", ErrStageNotPaid, stage)
	}

	switch stage {
	case StageVerification:
		e.VerificationPaid = false
	case StageJoin:
		e.JoinPaid = false
	case StageReferee:
		e.RefereePaid = false
	}

	if stage != StageReferee {
		e.CoinsAwarded -= e.Coins(stage)
	}

	return nil
}

// DeferNext moves the event's next check forward after a pass found nothing to
// pay for it, and returns that moment. The delay grows with every fruitless
// check up to checkBackoff's last step, so a referral that is waiting on a player
// who never comes costs one look an hour instead of one a minute.
func (e *ReferralEvent) DeferNext(now time.Time) time.Time {
	step := e.Checks
	if step >= len(checkBackoff) {
		step = len(checkBackoff) - 1
	}

	e.Checks++
	e.NextCheckAt = now.Add(checkBackoff[step])

	return e.NextCheckAt
}
