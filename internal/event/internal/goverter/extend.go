package goverter

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// TimestampToTimePtr keeps an absent proto timestamp absent: a missing ends_at
// means "no end", not the Unix epoch that (*Timestamp)(nil).AsTime() returns.
func TimestampToTimePtr(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	v := t.AsTime()
	return &v
}
