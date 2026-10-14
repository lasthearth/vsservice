package repository

import (
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/regexsafe"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// fixedNow is the clock every boundary in these tests is measured against, so a
// window is asserted against a number rather than against time.Now.
var fixedNow = time.Date(2026, time.March, 14, 12, 0, 0, 0, time.UTC)

// stage returns the body of the first stage with the given name.
func stage(t *testing.T, pipeline bson.A, name string) any {
	t.Helper()

	for _, raw := range pipeline {
		d, ok := raw.(bson.D)
		if !ok || len(d) == 0 || d[0].Key != name {
			continue
		}
		return d[0].Value
	}

	t.Fatalf("pipeline has no %s stage", name)
	return nil
}

// stageDoc is stage for the stages that carry a document.
func stageDoc(t *testing.T, pipeline bson.A, name string) bson.D {
	t.Helper()

	body, ok := stage(t, pipeline, name).(bson.D)
	if !ok {
		t.Fatalf("stage %s body is %T, want bson.D", name, stage(t, pipeline, name))
	}
	return body
}

// field returns one value of a document.
func field(t *testing.T, doc bson.D, key string) any {
	t.Helper()

	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}

	t.Fatalf("document has no %q field", key)
	return nil
}

// facetStage returns one of the three $facet branches.
func facetStage(t *testing.T, pipeline bson.A, name string) bson.A {
	t.Helper()

	branch, ok := field(t, stageDoc(t, pipeline, "$facet"), name).(bson.A)
	if !ok {
		t.Fatalf("facet %s is not a stage list", name)
	}
	return branch
}

func TestListPlayersPipelineEscapesTheQuery(t *testing.T) {
	pipeline := buildListPlayersPipeline(model.PlayerListFilter{Query: "ве.*т"}, 1, 24, fixedNow)

	match, ok := stage(t, pipeline, "$match").(bson.M)
	if !ok {
		t.Fatalf("$match = %T, want bson.M", stage(t, pipeline, "$match"))
	}

	statuses, ok := fieldM(match, "status").(bson.M)
	if !ok {
		t.Fatalf("status filter is %T, want bson.M", fieldM(match, "status"))
	}
	want := bson.A{
		string(verification.VerificationStatusVerified),
		string(verification.VerificationStatusApproved),
	}
	if got := statuses["$in"]; !equalAny(got, want) {
		t.Errorf("status $in = %v, want %v", got, want)
	}

	ors, ok := match["$or"].(bson.A)
	if !ok || len(ors) != 2 {
		t.Fatalf("$or = %v, want two nickname conditions", match["$or"])
	}

	escaped := regexsafe.Escape("ве.*т")
	if escaped == "ве.*т" {
		t.Fatal("the test query must contain metacharacters")
	}

	for _, raw := range ors {
		condition, ok := raw.(bson.M)
		if !ok || len(condition) != 1 {
			t.Fatalf("nickname condition = %v, want one field", raw)
		}
		for _, value := range condition {
			pattern, ok := value.(bson.M)
			if !ok {
				t.Fatalf("pattern = %T, want bson.M", value)
			}
			if pattern["$regex"] != escaped {
				t.Errorf("$regex = %v, want %q", pattern["$regex"], escaped)
			}
			if pattern["$options"] != "i" {
				t.Errorf("$options = %v, want i", pattern["$options"])
			}
		}
	}
}

// TestListPlayersPipelineJoinsStatsByExpression pins the join to the form the
// MongoDB manual documents. localField and foreignField are mutually exclusive
// with pipeline, so a server that enforces that would fail every roster request,
// and every other test here would stay green because none of them runs a query.
func TestListPlayersPipelineJoinsStatsByExpression(t *testing.T) {
	pipeline := buildListPlayersPipeline(model.PlayerListFilter{}, 1, 24, fixedNow)

	lookup := stageDoc(t, pipeline, "$lookup")

	for _, banned := range []string{"localField", "foreignField"} {
		if _, ok := getField(lookup, banned); ok {
			t.Errorf("$lookup carries %q, which MongoDB does not allow next to pipeline", banned)
		}
	}

	if _, ok := getField(lookup, "let"); !ok {
		t.Error("$lookup has no let, so the sub-pipeline cannot see the game name")
	}

	sub, ok := getField(lookup, "pipeline")
	if !ok {
		t.Fatal("$lookup has no pipeline")
	}
	stages, ok := sub.(bson.A)
	if !ok || len(stages) != 2 {
		t.Fatalf("$lookup pipeline = %v, want a $match and a $group", sub)
	}

	matchStage, ok := stages[0].(bson.D)
	if !ok || len(matchStage) == 0 || matchStage[0].Key != "$match" {
		t.Fatalf("the first sub-pipeline stage is %v, want the $expr join condition", stages[0])
	}
	expr, ok := matchStage[0].Value.(bson.D)
	if !ok || len(expr) == 0 || expr[0].Key != "$expr" {
		t.Fatalf("the join $match is %v, want an $expr equality on the game name", matchStage[0].Value)
	}
}

func TestListPlayersPipelineDropsNamelessApplications(t *testing.T) {
	pipeline := buildListPlayersPipeline(model.PlayerListFilter{}, 1, 24, fixedNow)

	match, ok := stage(t, pipeline, "$match").(bson.M)
	if !ok {
		t.Fatalf("$match = %T, want bson.M", stage(t, pipeline, "$match"))
	}

	names, ok := match["user_game_name"].(bson.M)
	if !ok {
		t.Fatalf("user_game_name filter = %T, want bson.M", match["user_game_name"])
	}
	if _, ok := names["$nin"]; !ok {
		t.Fatalf("user_game_name filter = %v, want a $nin that drops the empty and the missing", names)
	}

	// And an application the game never saw is dropped after the join.
	for _, raw := range pipeline {
		d, ok := raw.(bson.D)
		if !ok || len(d) == 0 || d[0].Key != "$match" {
			continue
		}
		body, ok := d[0].Value.(bson.M)
		if !ok {
			continue
		}
		ors, ok := body["$or"].(bson.A)
		if !ok || len(ors) != 2 {
			continue
		}
		if _, ok := ors[1].(bson.M)["has_stats"]; ok {
			return
		}
	}

	t.Error("no stage drops an approved application without stats")
}

func TestListPlayersPipelineWindowsNest(t *testing.T) {
	day := fixedNow.Add(-24 * time.Hour).UnixMilli()
	week := fixedNow.Add(-7 * 24 * time.Hour).UnixMilli()
	month := fixedNow.Add(-30 * 24 * time.Hour).UnixMilli()

	cases := []struct {
		name   string
		window model.LastSeenWindow
		want   bson.M
	}{
		{
			name:   "every player is no filter at all",
			window: model.LastSeenAny,
			want:   bson.M{},
		},
		{
			name:   "online is the game's own flag",
			window: model.LastSeenOnline,
			want:   bson.M{"is_online": true},
		},
		{
			name:   "a day includes who is online now",
			window: model.LastSeenDay,
			want: bson.M{"$or": bson.A{
				bson.M{"is_online": true},
				bson.M{"last_online": bson.M{"$gte": day}},
			}},
		},
		{
			name:   "a week includes who is online now",
			window: model.LastSeenWeek,
			want: bson.M{"$or": bson.A{
				bson.M{"is_online": true},
				bson.M{"last_online": bson.M{"$gte": week}},
			}},
		},
		{
			name:   "a month includes who is online now",
			window: model.LastSeenMonth,
			want: bson.M{"$or": bson.A{
				bson.M{"is_online": true},
				bson.M{"last_online": bson.M{"$gte": month}},
			}},
		},
		{
			name:   "inactive is everything the month leaves out",
			window: model.LastSeenInactive,
			want: bson.M{"$and": bson.A{
				bson.M{"is_online": bson.M{"$ne": true}},
				bson.M{"$or": bson.A{
					bson.M{"last_online": bson.M{"$lt": month}},
					bson.M{"last_online": bson.M{"$exists": false}},
					bson.M{"last_online": nil},
				}},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pipeline := buildListPlayersPipeline(
				model.PlayerListFilter{LastSeen: tc.window}, 1, 24, fixedNow)

			items := facetStage(t, pipeline, "items")
			got, ok := items[0].(bson.D)[0].Value.(bson.M)
			if !ok {
				t.Fatalf("items $match = %T, want bson.M", items[0].(bson.D)[0].Value)
			}
			if !equalAny(got, tc.want) {
				t.Errorf("items $match = %v, want %v", got, tc.want)
			}

			if tc.window == model.LastSeenAny {
				return
			}

			// The same window narrows the total, so total_size and players agree.
			total := facetStage(t, pipeline, "total")
			gotTotal := total[0].(bson.D)[0].Value.(bson.M)
			if !equalAny(gotTotal, tc.want) {
				t.Errorf("total $match = %v, want %v", gotTotal, tc.want)
			}
		})
	}
}

func TestListPlayersPipelineCountsEveryWindow(t *testing.T) {
	pipeline := buildListPlayersPipeline(model.PlayerListFilter{}, 1, 24, fixedNow)

	counts := facetStage(t, pipeline, "counts")
	group := counts[0].(bson.D)[0].Value.(bson.D)

	for _, key := range []string{"all", "online", "day", "week", "month", "inactive"} {
		if _, ok := getField(group, key); !ok {
			t.Errorf("counts stage has no %q", key)
		}
	}

	// Every count is a $cond over the same nesting the $match uses: a day
	// counts who is online now as well.
	day := getValue(t, group, "day")
	body := day.(bson.D)[0].Value.(bson.D)
	cond := getValue(t, body, "$cond").(bson.A)
	if !equalAny(cond[0].(bson.D)[0].Value, bson.A{
		onlineExpr(), seenSinceExpr(fixedNow.Add(-24 * time.Hour).UnixMilli()),
	}) {
		t.Errorf("day condition = %v, want online-or-within-24h", cond[0])
	}

	inactive := getValue(t, group, "inactive")
	inactiveBody := inactive.(bson.D)[0].Value.(bson.D)
	inactiveCond := getValue(t, inactiveBody, "$cond").(bson.A)
	if !equalAny(inactiveCond[0], inactiveExpr(fixedNow.Add(-30*24*time.Hour).UnixMilli())) {
		t.Errorf("inactive condition = %v, want outside the month", inactiveCond[0])
	}
}

func TestListPlayersPipelineSorts(t *testing.T) {
	cases := []struct {
		sort model.PlayerListSort
		want bson.D
	}{
		{
			// Online first, then the most recently seen; a player the game never
			// recorded has no date and sorts last.
			sort: model.PlayerSortLastSeen,
			want: bson.D{
				{Key: "is_online", Value: -1},
				{Key: "last_online", Value: -1},
				{Key: nicknameSortKey, Value: 1},
				{Key: "_id", Value: 1},
			},
		},
		{
			sort: model.PlayerSortHours,
			want: bson.D{
				{Key: "hours", Value: -1},
				{Key: nicknameSortKey, Value: 1},
				{Key: "_id", Value: 1},
			},
		},
		{
			sort: model.PlayerSortNickname,
			want: bson.D{{Key: nicknameSortKey, Value: 1}, {Key: "_id", Value: 1}},
		},
	}

	for _, tc := range cases {
		pipeline := buildListPlayersPipeline(model.PlayerListFilter{Sort: tc.sort}, 1, 24, fixedNow)
		items := facetStage(t, pipeline, "items")

		var got bson.D
		for _, raw := range items {
			d, ok := raw.(bson.D)
			if ok && len(d) > 0 && d[0].Key == "$sort" {
				got = d[0].Value.(bson.D)
			}
		}

		if !equalAny(got, tc.want) {
			t.Errorf("sort %d = %v, want %v", tc.sort, got, tc.want)
		}
	}
}

func TestListPlayersPipelinePages(t *testing.T) {
	cases := []struct {
		page, size int
		wantSkip   int64
		wantLimit  int64
	}{
		{page: 1, size: 24, wantSkip: 0, wantLimit: 24},
		{page: 2, size: 24, wantSkip: 24, wantLimit: 24},
		{page: 5, size: 48, wantSkip: 192, wantLimit: 48},
		{page: 0, size: 24, wantSkip: 0, wantLimit: 24},
	}

	for _, tc := range cases {
		pipeline := buildListPlayersPipeline(model.PlayerListFilter{}, tc.page, tc.size, fixedNow)
		items := facetStage(t, pipeline, "items")

		var skip, limit int64
		for _, raw := range items {
			d, ok := raw.(bson.D)
			if !ok || len(d) == 0 {
				continue
			}
			switch d[0].Key {
			case "$skip":
				skip = d[0].Value.(int64)
			case "$limit":
				limit = d[0].Value.(int64)
			}
		}

		if skip != tc.wantSkip || limit != tc.wantLimit {
			t.Errorf("page %d size %d: skip %d limit %d, want %d/%d",
				tc.page, tc.size, skip, limit, tc.wantSkip, tc.wantLimit)
		}
	}
}

func TestListPlayersPipelineProjectsTheRosterFields(t *testing.T) {
	pipeline := buildListPlayersPipeline(model.PlayerListFilter{}, 1, 24, fixedNow)
	items := facetStage(t, pipeline, "items")

	var projection bson.D
	for _, raw := range items {
		d, ok := raw.(bson.D)
		if ok && len(d) > 0 && d[0].Key == "$project" {
			projection = d[0].Value.(bson.D)
		}
	}

	for _, key := range []string{
		"user_id", "nickname", "avatar", "is_online", "last_online", "hours_played", "about",
	} {
		if _, ok := getField(projection, key); !ok {
			t.Errorf("projection has no %q", key)
		}
	}
}

// fieldM returns one value of a map document.
func fieldM(doc bson.M, key string) any {
	return doc[key]
}

func getField(doc bson.D, key string) (any, bool) {
	for _, e := range doc {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

func getValue(t *testing.T, doc bson.D, key string) any {
	t.Helper()
	v, ok := getField(doc, key)
	if !ok {
		t.Fatalf("document has no %q field", key)
	}
	return v
}

// equalAny compares the values a pipeline is built from, including nested
// documents and slices.
func equalAny(a, b any) bool {
	switch av := a.(type) {
	case bson.M:
		bv, ok := b.(bson.M)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !equalAny(v, bv[k]) {
				return false
			}
		}
		return true
	case bson.D:
		bv, ok := b.(bson.D)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i].Key != bv[i].Key || !equalAny(av[i].Value, bv[i].Value) {
				return false
			}
		}
		return true
	case bson.A:
		bv, ok := b.(bson.A)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equalAny(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
