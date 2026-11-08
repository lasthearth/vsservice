package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/ticket/internal/ierror"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPageTokenRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		field string
		at    time.Time
	}{
		{"staff inbox", "last_message_at", time.UnixMilli(1762600000123).UTC()},
		{"player list", "last_public_at", time.UnixMilli(86400000).UTC()},
		{"before the epoch", "last_public_at", time.UnixMilli(-1762600000123).UTC()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oid := bson.NewObjectID()
			token := pageToken(tc.field, tc.at, oid)

			want := tc.field + "|" + strconv.FormatInt(tc.at.UnixMilli(), 10) + "_" + oid.Hex()
			if token != want {
				t.Fatalf("token = %q, want %q", token, want)
			}

			at, id, err := parsePageToken(token, tc.field)
			if err != nil {
				t.Fatal(err)
			}
			if !at.Equal(tc.at) {
				t.Fatalf("at = %v (%d ms), want %v (%d ms)", at, at.UnixMilli(), tc.at, tc.at.UnixMilli())
			}
			if id != oid {
				t.Fatalf("id = %s, want %s", id.Hex(), oid.Hex())
			}
		})
	}
}

func TestParsePageTokenRejects(t *testing.T) {
	oid := bson.NewObjectID()
	crossView := pageToken("last_public_at", time.UnixMilli(1762600000123).UTC(), oid)

	tests := []struct {
		name  string
		token string
		field string
	}{
		{"no separator", "last_message_at1762600000123_" + oid.Hex(), "last_message_at"},
		{"non-numeric timestamp", "last_message_at|soon_" + oid.Hex(), "last_message_at"},
		{"bad hex id", "last_message_at|1762600000123_" + strings.Repeat("z", 24), "last_message_at"},
		{"missing underscore", "last_message_at|1762600000123", "last_message_at"},
		// A token from the player list must not page the staff inbox: the two
		// sort by different fields, so the position would be wrong.
		{"token of the other list", crossView, "last_message_at"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			at, id, err := parsePageToken(tc.token, tc.field)
			if err == nil {
				t.Fatalf("parsePageToken(%q, %q) = %v, %v; want an error", tc.token, tc.field, at, id)
			}
		})
	}
}

func TestListAuthorViewRequiresAuthorId(t *testing.T) {
	// A zero Repository has nil collections: any query that reached Mongo
	// would panic instead of returning, so a clean error proves the access
	// rule runs first.
	r := &Repository{}
	_, _, err := r.List(context.Background(), model.Query{Viewer: model.ViewerAuthor})
	if !errors.Is(err, ierror.ErrAuthorRequired) {
		t.Fatalf("err = %v, want ErrAuthorRequired", err)
	}
}
