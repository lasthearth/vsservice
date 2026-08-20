package pagination

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The contract that matters is end-to-end: paging with the tokens List hands out
// must visit every matching document exactly once. A live MongoDB is not
// available here, so these tests page over an in-memory collection through the
// real resolve/page/cursorFilter code, using a matcher that understands exactly
// the filter shapes this package emits ($and, $or, $lt, $gt and equality).

type row struct {
	OID   bson.ObjectID `bson:"_id"`
	Coins int64         `bson:"coins"`
	State string        `bson:"state"`
}

func (r row) Id() bson.ObjectID { return r.OID }

// matches evaluates a filter this package produced against one row.
func matches(t *testing.T, r row, filter bson.M) bool {
	t.Helper()

	for key, cond := range filter {
		switch key {
		case "$and":
			for _, sub := range cond.(bson.A) {
				if !matches(t, r, sub.(bson.M)) {
					return false
				}
			}
		case "$or":
			hit := false
			for _, sub := range cond.(bson.A) {
				if matches(t, r, sub.(bson.M)) {
					hit = true
					break
				}
			}
			if !hit {
				return false
			}
		default:
			if !matchField(t, r, key, cond) {
				return false
			}
		}
	}

	return true
}

func matchField(t *testing.T, r row, key string, cond any) bool {
	t.Helper()

	cmp, ok := cond.(bson.M)
	if !ok {
		// Equality. Cursor tie branches carry a bson.RawValue.
		if rv, isRaw := cond.(bson.RawValue); isRaw {
			return compare(t, r, key, rv) == 0
		}
		return fieldValue(t, r, key) == cond
	}

	for op, want := range cmp {
		var sign int
		switch w := want.(type) {
		case bson.ObjectID:
			sign = slices.Compare(r.OID[:], w[:])
		case bson.RawValue:
			sign = compare(t, r, key, w)
		default:
			t.Fatalf("unsupported comparison value %T", want)
		}

		switch op {
		case "$lt":
			if sign >= 0 {
				return false
			}
		case "$gt":
			if sign <= 0 {
				return false
			}
		default:
			t.Fatalf("unsupported operator %s", op)
		}
	}

	return true
}

func fieldValue(t *testing.T, r row, key string) any {
	t.Helper()

	switch key {
	case "coins":
		return r.Coins
	case "state":
		return r.State
	default:
		t.Fatalf("unknown field %s", key)
		return nil
	}
}

// compare reports sign(row value - cursor value) for the coins field.
func compare(t *testing.T, r row, key string, want bson.RawValue) int {
	t.Helper()

	if key != "coins" {
		t.Fatalf("compare only handles coins, got %s", key)
	}

	var w int64
	if err := want.Unmarshal(&w); err != nil {
		t.Fatalf("unmarshal cursor value: %v", err)
	}

	switch {
	case r.Coins < w:
		return -1
	case r.Coins > w:
		return 1
	default:
		return 0
	}
}

func sortRows(t *testing.T, rows []row, sort bson.D) {
	t.Helper()

	slices.SortFunc(rows, func(a, b row) int {
		for _, key := range sort {
			dir := 1
			if !ascending(key.Value) {
				dir = -1
			}

			var sign int
			switch key.Key {
			case "_id":
				sign = slices.Compare(a.OID[:], b.OID[:])
			case "coins":
				sign = int(min(max(a.Coins-b.Coins, -1), 1))
			default:
				t.Fatalf("unknown sort key %s", key.Key)
			}

			if sign != 0 {
				return sign * dir
			}
		}
		return 0
	})
}

// pageThrough walks every page the way a client would and returns the ids it
// saw, in order, plus the number of round trips.
func pageThrough(t *testing.T, rows []row, limit int64, opts ...OptionFn) ([]bson.ObjectID, int) {
	t.Helper()

	var seen []bson.ObjectID
	next := ""
	trips := 0

	for {
		trips++
		if trips > 100 {
			t.Fatal("pagination did not terminate")
		}

		q, err := resolve(append(slices.Clone(opts), WithLimit(limit), WithNext(next)))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}

		var hits []row
		for _, r := range rows {
			if matches(t, r, q.filter) {
				hits = append(hits, r)
			}
		}
		sortRows(t, hits, q.sort)
		if int64(len(hits)) > q.limit+1 {
			hits = hits[:q.limit+1]
		}

		got, token, err := page(hits, q.limit, q.sort)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		for _, r := range got {
			seen = append(seen, r.OID)
		}

		if token == "" {
			return seen, trips
		}
		next = token
	}
}

func rowsWithDistinctCoins(n int) []row {
	out := make([]row, n)
	for i := range out {
		out[i] = row{OID: bson.NewObjectID(), Coins: int64(n - i), State: "active"}
	}
	return out
}

func assertVisitedOnce(t *testing.T, seen []bson.ObjectID, rows []row) {
	t.Helper()

	if len(seen) != len(rows) {
		t.Fatalf("visited %d documents, want %d", len(seen), len(rows))
	}

	unique := map[bson.ObjectID]int{}
	for _, id := range seen {
		unique[id]++
	}
	for id, n := range unique {
		if n != 1 {
			t.Fatalf("document %v returned %d times", id, n)
		}
	}
	for _, r := range rows {
		if _, ok := unique[r.OID]; !ok {
			t.Fatalf("document %v never returned", r.OID)
		}
	}
}

// The original bug: WithNext mutated the filter, WithFilter replaced it, so
// WithNext first dropped the cursor and every page repeated page 1.
func TestPagingVisitsEveryDocumentInEitherOptionOrder(t *testing.T) {
	rows := rowsWithDistinctCoins(7)
	filter := bson.M{"state": "active"}

	orders := map[string][]OptionFn{
		"filter only":  {WithFilter(filter)},
		"filter first": {WithFilter(filter), WithSort(bson.D{{Key: "coins", Value: -1}})},
		"sort first":   {WithSort(bson.D{{Key: "coins", Value: -1}}), WithFilter(filter)},
	}

	for name, opts := range orders {
		t.Run(name, func(t *testing.T) {
			seen, _ := pageThrough(t, rows, 3, opts...)
			assertVisitedOnce(t, seen, rows)
		})
	}
}

// A non-unique sort field is where an _id-only cursor loses documents: rows
// tying on coins must still be visited exactly once.
func TestPagingIsExactWithTiesOnTheSortField(t *testing.T) {
	rows := make([]row, 0, 9)
	for i := range 9 {
		rows = append(rows, row{OID: bson.NewObjectID(), Coins: int64(i / 3), State: "active"})
	}

	for _, dir := range []int{1, -1} {
		seen, _ := pageThrough(t, rows, 2,
			WithFilter(bson.M{"state": "active"}),
			WithSort(bson.D{{Key: "coins", Value: dir}}),
		)
		assertVisitedOnce(t, seen, rows)
	}
}

// Ascending order must page forwards, not restart from the top.
func TestPagingRespectsSortDirection(t *testing.T) {
	rows := rowsWithDistinctCoins(6)

	for _, dir := range []int{1, -1} {
		seen, _ := pageThrough(t, rows, 2, WithSort(bson.D{{Key: "coins", Value: dir}}))
		assertVisitedOnce(t, seen, rows)

		ordered := slices.Clone(rows)
		sortRows(t, ordered, bson.D{{Key: "coins", Value: dir}, {Key: "_id", Value: dir}})
		for i, r := range ordered {
			if seen[i] != r.OID {
				t.Fatalf("dir %d: position %d = %v, want %v", dir, i, seen[i], r.OID)
			}
		}
	}
}

// The limit+1 lookahead: an exactly-full last page must not cost an extra
// request. Six documents at limit 3 is two pages, not three.
func TestExhaustedListCostsNoExtraRoundTrip(t *testing.T) {
	seen, trips := pageThrough(t, rowsWithDistinctCoins(6), 3)

	if len(seen) != 6 {
		t.Fatalf("visited %d, want 6", len(seen))
	}
	if trips != 2 {
		t.Fatalf("%d round trips, want 2", trips)
	}
}

func TestEmptyCollectionIsOneTripAndNoError(t *testing.T) {
	seen, trips := pageThrough(t, nil, 3)

	if len(seen) != 0 {
		t.Fatalf("visited %d, want 0", len(seen))
	}
	if trips != 1 {
		t.Fatalf("%d round trips, want 1", trips)
	}
}
