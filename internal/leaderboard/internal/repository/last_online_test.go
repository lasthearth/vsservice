package repository

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func raw(t *testing.T, v any) bson.RawValue {
	t.Helper()
	typ, data, err := bson.MarshalValue(v)
	if err != nil {
		t.Fatal(err)
	}
	return bson.RawValue{Type: typ, Value: data}
}

func TestLastOnline(t *testing.T) {
	want := time.Date(2026, 10, 5, 12, 30, 0, 0, time.UTC)

	cases := map[string]bson.RawValue{
		"datetime": raw(t, bson.NewDateTimeFromTime(want)),
		"rfc3339":  raw(t, want.Format(time.RFC3339)),
		"seconds":  raw(t, want.Unix()),
		"millis":   raw(t, want.UnixMilli()),
		"int32":    raw(t, int32(want.Unix())),
	}
	for name, v := range cases {
		got := lastOnline(v)
		if got == nil || !got.Equal(want) {
			t.Errorf("%s: want %v, got %v", name, want, got)
		}
	}

	for name, v := range map[string]bson.RawValue{
		"missing": {},
		"null":    {Type: bson.TypeNull},
		"zero":    raw(t, int64(0)),
		"garbage": raw(t, "yesterday"),
		"ancient": raw(t, time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC).Unix()),
	} {
		if got := lastOnline(v); got != nil {
			t.Errorf("%s: want nil, got %v", name, got)
		}
	}
}
