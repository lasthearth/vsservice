package pagination

import (
	"encoding/base64"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type stub struct {
	OID   bson.ObjectID `bson:"_id"`
	Coins int64         `bson:"coins"`
}

func (s stub) Id() bson.ObjectID { return s.OID }

func stubs(n int) []stub {
	out := make([]stub, n)
	for i := range out {
		out[i] = stub{OID: bson.NewObjectID(), Coins: int64(n - i)}
	}
	return out
}

func rawInt64(t *testing.T, v int64) bson.RawValue {
	t.Helper()

	raw, err := bson.Marshal(bson.D{{Key: "v", Value: v}})
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}

	rv, err := bson.Raw(raw).LookupErr("v")
	if err != nil {
		t.Fatalf("LookupErr: %v", err)
	}

	return rv
}

func TestTokenRoundTrip(t *testing.T) {
	want := token{Oid: bson.NewObjectID(), Key: "coins", Val: rawInt64(t, 42)}

	encoded, err := encodeToken(want)
	if err != nil {
		t.Fatalf("encodeToken: %v", err)
	}
	if encoded == "" {
		t.Fatal("encodeToken returned an empty string")
	}

	got, err := decodeToken(encoded)
	if err != nil {
		t.Fatalf("decodeToken: %v", err)
	}
	if got.Oid != want.Oid {
		t.Fatalf("oid = %v, want %v", got.Oid, want.Oid)
	}
	if got.Key != want.Key {
		t.Fatalf("key = %q, want %q", got.Key, want.Key)
	}
	if !got.Val.Equal(want.Val) {
		t.Fatalf("val = %v, want %v", got.Val, want.Val)
	}
}

func TestTokenRoundTripIDOnly(t *testing.T) {
	want := token{Oid: bson.NewObjectID()}

	encoded, err := encodeToken(want)
	if err != nil {
		t.Fatalf("encodeToken: %v", err)
	}

	got, err := decodeToken(encoded)
	if err != nil {
		t.Fatalf("decodeToken: %v", err)
	}
	if got.Oid != want.Oid {
		t.Fatalf("oid = %v, want %v", got.Oid, want.Oid)
	}
	if got.Key != "" || !got.Val.IsZero() {
		t.Fatalf("unexpected primary key in token: %#v", got)
	}
}

// Tokens handed out before the format change must keep working, otherwise a
// client mid-pagination gets an error on deploy.
func TestDecodeTokenAcceptsLegacyJSON(t *testing.T) {
	oid := bson.NewObjectID()
	legacy := `{"oid":"` + oid.Hex() + `"}`

	got, err := decodeToken(base64.RawStdEncoding.EncodeToString([]byte(legacy)))
	if err != nil {
		t.Fatalf("decodeToken: %v", err)
	}
	if got.Oid != oid {
		t.Fatalf("oid = %v, want %v", got.Oid, oid)
	}
}

func TestDecodeTokenRejectsGarbage(t *testing.T) {
	if _, err := decodeToken("!!!not base64!!!"); err == nil {
		t.Fatal("expected an error for a non-base64 token")
	}
}

// The bug this package used to have: WithNext mutated o.filter, WithFilter
// replaced it, so WithNext before WithFilter silently dropped the cursor and
// page 2 returned page 1.
func TestCursorSurvivesAnyOptionOrder(t *testing.T) {
	oid := bson.NewObjectID()
	next, err := encodeToken(token{Oid: oid})
	if err != nil {
		t.Fatalf("encodeToken: %v", err)
	}

	filter := bson.M{"status": "active"}

	orders := map[string][]OptionFn{
		"next first":   {WithNext(next), WithFilter(filter)},
		"filter first": {WithFilter(filter), WithNext(next)},
	}

	for name, opts := range orders {
		t.Run(name, func(t *testing.T) {
			q, err := resolve(opts)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}

			and, ok := q.filter["$and"].(bson.A)
			if !ok || len(and) != 2 {
				t.Fatalf("filter = %#v, want the caller filter and the cursor under $and", q.filter)
			}

			caller, ok := and[0].(bson.M)
			if !ok || caller["status"] != "active" {
				t.Fatalf("caller filter lost: %#v", and[0])
			}

			cursor, ok := and[1].(bson.M)
			if !ok {
				t.Fatalf("cursor clause = %#v", and[1])
			}
			idCmp, ok := cursor["_id"].(bson.M)
			if !ok || idCmp["$lt"] != oid {
				t.Fatalf("cursor = %#v, want _id $lt %v", cursor, oid)
			}
		})
	}
}

func TestResolveDoesNotMutateCallerFilter(t *testing.T) {
	next, err := encodeToken(token{Oid: bson.NewObjectID()})
	if err != nil {
		t.Fatalf("encodeToken: %v", err)
	}

	filter := bson.M{"status": "active"}
	if _, err := resolve([]OptionFn{WithFilter(filter), WithNext(next)}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if len(filter) != 1 || filter["status"] != "active" {
		t.Fatalf("caller filter was mutated: %#v", filter)
	}
}

func TestResolveNoCursorLeavesFilterAlone(t *testing.T) {
	q, err := resolve([]OptionFn{WithFilter(bson.M{"status": "active"})})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if q.filter["status"] != "active" {
		t.Fatalf("filter = %#v", q.filter)
	}
	if _, ok := q.filter["$and"]; ok {
		t.Fatalf("unexpected $and without a cursor: %#v", q.filter)
	}
}

func TestResolveLimitDefaultAndClamp(t *testing.T) {
	cases := map[string]struct {
		requested int64
		want      int64
	}{
		"zero defaults":     {0, DefaultLimit},
		"negative defaults": {-5, DefaultLimit},
		"in range kept":     {10, 10},
		"max kept":          {MaxLimit, MaxLimit},
		"over max clamped":  {MaxLimit + 1, MaxLimit},
		"absurd clamped":    {1 << 20, MaxLimit},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			q, err := resolve([]OptionFn{WithLimit(tc.requested)})
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if q.limit != tc.want {
				t.Fatalf("limit = %d, want %d", q.limit, tc.want)
			}
		})
	}
}

func TestResolveSortAlwaysEndsWithID(t *testing.T) {
	cases := map[string]struct {
		sort    bson.D
		wantDir any
	}{
		"default":          {sort: nil, wantDir: -1},
		"descending field": {sort: bson.D{{Key: "coins", Value: -1}}, wantDir: -1},
		"ascending field":  {sort: bson.D{{Key: "created_at", Value: 1}}, wantDir: 1},
		"id already last":  {sort: bson.D{{Key: "coins", Value: 1}, {Key: "_id", Value: 1}}, wantDir: 1},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts := []OptionFn{}
			if tc.sort != nil {
				opts = append(opts, WithSort(tc.sort))
			}

			q, err := resolve(opts)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}

			last := q.sort[len(q.sort)-1]
			if last.Key != "_id" {
				t.Fatalf("sort = %#v, want _id last", q.sort)
			}
			if last.Value != tc.wantDir {
				t.Fatalf("_id direction = %v, want %v", last.Value, tc.wantDir)
			}
		})
	}
}

// An ascending sort must page forwards, a descending one backwards, otherwise
// page 2 re-reads page 1.
func TestCursorDirectionFollowsSort(t *testing.T) {
	if got := op(1); got != "$gt" {
		t.Fatalf("ascending: op = %s, want $gt", got)
	}
	if got := op(-1); got != "$lt" {
		t.Fatalf("descending: op = %s, want $lt", got)
	}
}

// Sorting by a non-unique field needs a compound comparison: an `_id`-only
// cursor would drop rows that sort after the cursor on the primary key but
// happen to carry a smaller `_id`.
func TestCursorFilterIsCompoundWhenSortingByAField(t *testing.T) {
	oid := bson.NewObjectID()
	val := rawInt64(t, 100)
	sort := bson.D{{Key: "coins", Value: -1}, {Key: "_id", Value: -1}}

	got := cursorFilter(sort, token{Oid: oid, Key: "coins", Val: val})

	or, ok := got["$or"].(bson.A)
	if !ok || len(or) != 2 {
		t.Fatalf("cursor = %#v, want a two-branch $or", got)
	}

	strictly, ok := or[0].(bson.M)
	if !ok {
		t.Fatalf("branch 0 = %#v", or[0])
	}
	cmp, ok := strictly["coins"].(bson.M)
	if !ok || !cmp["$lt"].(bson.RawValue).Equal(val) {
		t.Fatalf("branch 0 = %#v, want coins $lt %v", strictly, val)
	}

	tie, ok := or[1].(bson.M)
	if !ok {
		t.Fatalf("branch 1 = %#v", or[1])
	}
	if !tie["coins"].(bson.RawValue).Equal(val) {
		t.Fatalf("branch 1 = %#v, want the tie on coins", tie)
	}
	if idCmp, ok := tie["_id"].(bson.M); !ok || idCmp["$lt"] != oid {
		t.Fatalf("branch 1 _id = %#v, want $lt %v", tie["_id"], oid)
	}
}

// Sorting by `_id` alone, and legacy tokens that carry no primary key, use the
// plain `_id` comparison.
func TestCursorFilterFallsBackToID(t *testing.T) {
	oid := bson.NewObjectID()

	cases := map[string]struct {
		sort bson.D
		tok  token
	}{
		"id-only sort":      {bson.D{{Key: "_id", Value: -1}}, token{Oid: oid, Key: "coins", Val: rawInt64(t, 1)}},
		"token without key": {bson.D{{Key: "coins", Value: -1}, {Key: "_id", Value: -1}}, token{Oid: oid}},
		"stale token key":   {bson.D{{Key: "coins", Value: -1}, {Key: "_id", Value: -1}}, token{Oid: oid, Key: "created_at", Val: rawInt64(t, 1)}},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := cursorFilter(tc.sort, tc.tok)
			idCmp, ok := got["_id"].(bson.M)
			if !ok || idCmp["$lt"] != oid {
				t.Fatalf("cursor = %#v, want a plain _id $lt", got)
			}
		})
	}
}

func TestPageEmitsCursorOnlyOnLookahead(t *testing.T) {
	const limit = 3
	idSort := bson.D{{Key: "_id", Value: -1}}

	t.Run("full page with lookahead", func(t *testing.T) {
		fetched := stubs(limit + 1)

		got, next, err := page(fetched, limit, idSort)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		if len(got) != limit {
			t.Fatalf("len = %d, want %d", len(got), limit)
		}
		if next == "" {
			t.Fatal("expected a cursor when the lookahead document came back")
		}

		decoded, err := decodeToken(next)
		if err != nil {
			t.Fatalf("decodeToken: %v", err)
		}
		if decoded.Oid != fetched[limit-1].OID {
			t.Fatalf("cursor points at %v, want the last returned document %v", decoded.Oid, fetched[limit-1].OID)
		}
	})

	// The wasted round trip the old contract caused: a full last page handed out
	// a cursor whose page was empty.
	t.Run("exactly full last page emits no cursor", func(t *testing.T) {
		got, next, err := page(stubs(limit), limit, idSort)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		if len(got) != limit {
			t.Fatalf("len = %d, want %d", len(got), limit)
		}
		if next != "" {
			t.Fatalf("next = %q, want no cursor", next)
		}
	})

	t.Run("partial page emits no cursor", func(t *testing.T) {
		got, next, err := page(stubs(limit-1), limit, idSort)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		if len(got) != limit-1 {
			t.Fatalf("len = %d, want %d", len(got), limit-1)
		}
		if next != "" {
			t.Fatalf("next = %q, want no cursor", next)
		}
	})
}

// The cursor must carry the primary sort field's value off the last returned
// document, not off the lookahead one.
func TestPageCursorCarriesPrimarySortValue(t *testing.T) {
	const limit = 2
	fetched := stubs(limit + 1)
	sort := bson.D{{Key: "coins", Value: -1}, {Key: "_id", Value: -1}}

	_, next, err := page(fetched, limit, sort)
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	decoded, err := decodeToken(next)
	if err != nil {
		t.Fatalf("decodeToken: %v", err)
	}
	if decoded.Key != "coins" {
		t.Fatalf("key = %q, want coins", decoded.Key)
	}

	var coins int64
	if err := decoded.Val.Unmarshal(&coins); err != nil {
		t.Fatalf("unmarshal cursor value: %v", err)
	}
	if coins != fetched[limit-1].Coins {
		t.Fatalf("coins = %d, want %d", coins, fetched[limit-1].Coins)
	}
}

// A sort field the document does not carry degrades to an _id-only cursor
// instead of failing.
func TestPageUnknownSortFieldDegradesToID(t *testing.T) {
	const limit = 1
	fetched := stubs(limit + 1)

	_, next, err := page(fetched, limit, bson.D{{Key: "nope", Value: -1}, {Key: "_id", Value: -1}})
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	decoded, err := decodeToken(next)
	if err != nil {
		t.Fatalf("decodeToken: %v", err)
	}
	if !decoded.Val.IsZero() {
		t.Fatalf("val = %v, want zero", decoded.Val)
	}
	if got := cursorFilter(bson.D{{Key: "nope", Value: -1}, {Key: "_id", Value: -1}}, decoded); got["$or"] != nil {
		t.Fatalf("cursor = %#v, want a plain _id comparison", got)
	}
}

// An empty page is a normal outcome, not an error.
func TestPageEmptyIsNotAnError(t *testing.T) {
	got, next, err := page([]stub{}, DefaultLimit, bson.D{{Key: "_id", Value: -1}})
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
	if next != "" {
		t.Fatalf("next = %q, want no cursor", next)
	}
}

// List's conversion step: nothing decoded yields a non-nil empty slice, an
// empty cursor and no error, so callers need no ErrNoData guard.
func TestConvertEmptyYieldsEmptySlice(t *testing.T) {
	datas, next, err := page[stub](nil, DefaultLimit, bson.D{{Key: "_id", Value: -1}})
	if err != nil {
		t.Fatalf("page: %v", err)
	}

	items := make([]string, len(datas))
	for i, d := range datas {
		items[i] = d.Id().Hex()
	}

	if items == nil {
		t.Fatal("items = nil, want an empty slice")
	}
	if len(items) != 0 || next != "" {
		t.Fatalf("items = %#v, next = %q", items, next)
	}
}

func TestSortValueReadsTimeFields(t *testing.T) {
	type doc struct {
		OID       bson.ObjectID `bson:"_id"`
		CreatedAt time.Time     `bson:"created_at"`
	}

	d := doc{OID: bson.NewObjectID(), CreatedAt: time.Now()}

	if got := sortValue(d, "created_at"); got.IsZero() {
		t.Fatal("created_at read as zero")
	}
	if got := sortValue(d, "missing"); !got.IsZero() {
		t.Fatalf("missing field = %v, want zero", got)
	}
}
