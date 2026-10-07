package service

import (
	"time"

	lfgv1 "github.com/lasthearth/vsservice/gen/lfg/v1"
	"github.com/lasthearth/vsservice/internal/lfg/internal/model"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var activities = map[model.Activity]lfgv1.Activity{
	model.ActivityMining:    lfgv1.Activity_ACTIVITY_MINING,
	model.ActivityExploring: lfgv1.Activity_ACTIVITY_EXPLORING,
	model.ActivityBuilding:  lfgv1.Activity_ACTIVITY_BUILDING,
	model.ActivityHunting:   lfgv1.Activity_ACTIVITY_HUNTING,
	model.ActivityTrading:   lfgv1.Activity_ACTIVITY_TRADING,
	model.ActivityFarming:   lfgv1.Activity_ACTIVITY_FARMING,
	model.ActivityCombat:    lfgv1.Activity_ACTIVITY_COMBAT,
	model.ActivityOther:     lfgv1.Activity_ACTIVITY_OTHER,
}

var kinds = map[model.Kind]lfgv1.PostKind{
	model.KindSession:  lfgv1.PostKind_POST_KIND_SESSION,
	model.KindTeammate: lfgv1.PostKind_POST_KIND_TEAMMATE,
}

var playTimes = map[model.PlayTime]lfgv1.PlayTime{
	model.PlayTimeMorning: lfgv1.PlayTime_PLAY_TIME_MORNING,
	model.PlayTimeDay:     lfgv1.PlayTime_PLAY_TIME_DAY,
	model.PlayTimeEvening: lfgv1.PlayTime_PLAY_TIME_EVENING,
	model.PlayTimeNight:   lfgv1.PlayTime_PLAY_TIME_NIGHT,
}

var experiences = map[model.Experience]lfgv1.Experience{
	model.ExperienceNewbie:      lfgv1.Experience_EXPERIENCE_NEWBIE,
	model.ExperienceExperienced: lfgv1.Experience_EXPERIENCE_EXPERIENCED,
	model.ExperienceVeteran:     lfgv1.Experience_EXPERIENCE_VETERAN,
}

// reverse finds the model value of a proto enum; zero when unset or unknown,
// which the model rejects (or, for experience, reads as "not said").
func reverse[M comparable, P comparable](table map[M]P, p P) M {
	for m, v := range table {
		if v == p {
			return m
		}
	}
	var zero M
	return zero
}

// ActivityToProto converts a model activity to its proto enum.
func ActivityToProto(a model.Activity) lfgv1.Activity { return activities[a] }

// ActivityFromProto converts a proto activity to the model.
func ActivityFromProto(a lfgv1.Activity) model.Activity { return reverse(activities, a) }

// KindToProto converts a model kind to its proto enum.
func KindToProto(k model.Kind) lfgv1.PostKind { return kinds[k] }

// KindFromProto converts a proto kind to the model.
func KindFromProto(k lfgv1.PostKind) model.Kind { return reverse(kinds, k) }

// PlayTimeToProto converts a model part of the day to its proto enum.
func PlayTimeToProto(t model.PlayTime) lfgv1.PlayTime { return playTimes[t] }

// PlayTimeFromProto converts a proto part of the day to the model.
func PlayTimeFromProto(t lfgv1.PlayTime) model.PlayTime { return reverse(playTimes, t) }

// ExperienceToProto converts a model experience to its proto enum.
func ExperienceToProto(e model.Experience) lfgv1.Experience { return experiences[e] }

// ExperienceFromProto converts a proto experience to the model.
func ExperienceFromProto(e lfgv1.Experience) model.Experience { return reverse(experiences, e) }

// TimestampToTimePtr converts an optional timestamp; nil stays nil.
func TimestampToTimePtr(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	v := t.AsTime()
	return &v
}

// IsClosed reports whether the author closed the post.
func IsClosed(closedAt *time.Time) bool {
	return closedAt != nil
}

// HasContact reports whether the author left a contact.
func HasContact(contact string) bool {
	return contact != ""
}
