package model

import (
	"crypto/rand"
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
	PlayerName string // Needed later for donateuc.AddCoins(playerID, playerName, amount).
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

// ReferralEvent records that a referee player was successfully referred by a
// referrer player, and how many coins the referrer was awarded for it.
type ReferralEvent struct {
	Id               string
	ReferrerPlayerID string
	RefereePlayerID  string
	CoinsAwarded     int64
	CreatedAt        time.Time
}

// NewReferralEvent creates a new ReferralEvent recording that refereePlayerID
// was referred by referrerPlayerID, awarding coinsAwarded coins.
func NewReferralEvent(referrerPlayerID, refereePlayerID string, coinsAwarded int64) *ReferralEvent {
	return &ReferralEvent{
		ReferrerPlayerID: referrerPlayerID,
		RefereePlayerID:  refereePlayerID,
		CoinsAwarded:     coinsAwarded,
		CreatedAt:        time.Now(),
	}
}

// ReconstituteReferralEvent rebuilds a ReferralEvent from persisted state. Repository use only.
func ReconstituteReferralEvent(id, referrerPlayerID, refereePlayerID string, coinsAwarded int64, createdAt time.Time) *ReferralEvent {
	return &ReferralEvent{
		Id:               id,
		ReferrerPlayerID: referrerPlayerID,
		RefereePlayerID:  refereePlayerID,
		CoinsAwarded:     coinsAwarded,
		CreatedAt:        createdAt,
	}
}
