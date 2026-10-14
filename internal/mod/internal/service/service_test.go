package service_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/mod/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/mod/internal/service"
	"github.com/lasthearth/vsservice/internal/mod/internal/service/sermapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/moddb"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

// fixedNow is the clock the stored write times are taken from.
var fixedNow = time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)

// fakeRepo keeps the mods in memory and applies the writes the way the mongo
// repository does: Create puts the mod first, Save changes what the callback
// changes.
type fakeRepo struct {
	mods    []model.Mod
	created []model.Mod
	saved   []model.Mod
	order   [][]string
}

func (r *fakeRepo) List(context.Context) ([]model.Mod, error) {
	// The real repository reads the list in display order; the fake has to sort
	// for the same reason a reorder is visible at all.
	sorted := append([]model.Mod(nil), r.mods...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })

	return sorted, nil
}

func (r *fakeRepo) ListRequired(ctx context.Context) ([]model.Mod, error) {
	all, err := r.List(ctx)
	if err != nil {
		return nil, err
	}

	required := make([]model.Mod, 0, len(all))
	for _, mod := range all {
		if mod.Audience == model.AudienceRequired {
			required = append(required, mod)
		}
	}

	return required, nil
}

func (r *fakeRepo) Get(_ context.Context, modId string) (*model.Mod, error) {
	for i := range r.mods {
		if r.mods[i].ModId == modId {
			stored := r.mods[i]
			return &stored, nil
		}
	}

	return nil, ierror.ErrNotInList
}

func (r *fakeRepo) Create(_ context.Context, mod *model.Mod) error {
	added := *mod
	added.SetPosition(0)
	r.created = append(r.created, added)
	r.mods = append([]model.Mod{added}, r.mods...)

	return nil
}

func (r *fakeRepo) Save(
	_ context.Context,
	modId string,
	fn func(ctx context.Context, mod *model.Mod) (*model.Mod, error),
) (*model.Mod, error) {
	for i := range r.mods {
		if r.mods[i].ModId != modId {
			continue
		}

		updated, err := fn(context.Background(), &r.mods[i])
		if err != nil {
			return nil, err
		}

		r.saved = append(r.saved, *updated)
		return updated, nil
	}

	return nil, ierror.ErrNotInList
}

func (r *fakeRepo) Delete(_ context.Context, modId string) error {
	for i := range r.mods {
		if r.mods[i].ModId == modId {
			r.mods = append(r.mods[:i], r.mods[i+1:]...)
			return nil
		}
	}

	return ierror.ErrNotInList
}

func (r *fakeRepo) Reorder(_ context.Context, ids []string) error {
	r.order = append(r.order, ids)

	for position, id := range ids {
		for i := range r.mods {
			if r.mods[i].ModId == id {
				r.mods[i].SetPosition(position)
			}
		}
	}

	return nil
}

// fakeModDB stands in for ModDB.
type fakeModDB struct {
	mod  *moddb.Mod
	err  error
	size int64
	refs []string
}

func (f *fakeModDB) GetMod(_ context.Context, ref string) (*moddb.Mod, error) {
	f.refs = append(f.refs, ref)
	if f.err != nil {
		return nil, f.err
	}

	return f.mod, nil
}

func (f *fakeModDB) FileSize(context.Context, string) int64 { return f.size }

// modDBFixture is one mod as ModDB answers, with two releases newest first.
func modDBFixture() *moddb.Mod {
	return &moddb.Mod{
		AssetID:  4405,
		Name:     "Carry On",
		Author:   "NerdScurvy",
		Text:     "<p>Carry On lets you pick up placed blocks.</p><p>And then some.</p>",
		LogoFile: "https://moddbcdn.vintagestory.at/CarryOn.png",
		URLAlias: "carryon",
		Side:     "both",
		Releases: []moddb.Release{
			{
				ModIDStr:   "carryon",
				ModVersion: "2.0.0",
				Tags:       []string{"1.21.5"},
				MainFile:   "https://moddbcdn.vintagestory.at/CarryOn-2.0.0.zip",
				FileName:   "CarryOn-2.0.0.zip",
				Created:    "2026-10-14 13:43:04",
			},
			{
				ModIDStr:   "carryon",
				ModVersion: "1.0.0",
				Tags:       []string{"1.21.4"},
				MainFile:   "https://moddbcdn.vintagestory.at/CarryOn-1.0.0.zip",
				FileName:   "CarryOn-1.0.0.zip",
				Created:    "2026-01-02 10:00:00",
			},
		},
	}
}

func newService(t *testing.T, repo service.Repository, client service.ModDB) *service.Service {
	t.Helper()

	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	return service.New(service.Opts{Logger: l, Repo: repo, Mapper: &sermapper.MapperImpl{}, ModDB: client})
}

// storedMod is one mod already on the list.
func storedMod(modId string, audience model.Audience, version string) model.Mod {
	release := model.Release{
		Version:  version,
		FileURL:  "https://moddbcdn.vintagestory.at/" + modId + "-" + version + ".zip",
		FileName: modId + "-" + version + ".zip",
		FileSize: 1024,
	}

	mod := model.Build(modId, model.Snapshot{
		Name:     modId,
		Releases: []model.Release{release},
	}, audience, "", &release, fixedNow)

	return mod
}

func TestLookupModOffersTheNewestVersion(t *testing.T) {
	svc := newService(t, &fakeRepo{}, &fakeModDB{mod: modDBFixture()})

	got, err := svc.LookupMod(context.Background(), &modv1.LookupModRequest{Query: "carryon"})
	if err != nil {
		t.Fatalf("LookupMod: %v", err)
	}

	mod := got.GetMod()
	if mod.GetId() != "carryon" || mod.GetName() != "Carry On" {
		t.Errorf("mod = %s / %s", mod.GetId(), mod.GetName())
	}
	if mod.GetAudience() != modv1.ModAudience_MOD_AUDIENCE_UNSPECIFIED {
		t.Errorf("audience = %v, want UNSPECIFIED: a lookup stores nothing", mod.GetAudience())
	}
	if mod.GetRelease() != nil {
		t.Errorf("release = %v, want unset: no version was chosen", mod.GetRelease())
	}
	if mod.GetLatestVersion() != "2.0.0" {
		t.Errorf("latest_version = %q, want the newest ModDB version", mod.GetLatestVersion())
	}
	if mod.GetSummary() != "Carry On lets you pick up placed blocks." {
		t.Errorf("summary = %q", mod.GetSummary())
	}
	if mod.GetModdbUrl() != "https://mods.vintagestory.at/carryon" {
		t.Errorf("moddb_url = %q", mod.GetModdbUrl())
	}
	if mod.GetSide() != modv1.ModSide_MOD_SIDE_BOTH {
		t.Errorf("side = %v, want BOTH", mod.GetSide())
	}
	if len(got.GetReleases()) != 2 {
		t.Fatalf("releases = %d, want 2", len(got.GetReleases()))
	}
	if got.GetReleases()[0].GetVersion() != "2.0.0" {
		t.Errorf("first release = %q, want the newest first", got.GetReleases()[0].GetVersion())
	}
	if mod.GetReleases() != nil {
		t.Error("the lookup answer must not repeat the releases inside the mod")
	}
}

func TestAddModGoesFirstAndKeepsTheChosenVersion(t *testing.T) {
	repo := &fakeRepo{mods: []model.Mod{storedMod("existing", model.AudienceRequired, "1.0.0")}}
	svc := newService(t, repo, &fakeModDB{mod: modDBFixture(), size: 659496})

	got, err := svc.AddMod(context.Background(), &modv1.AddModRequest{
		Query:    "https://mods.vintagestory.at/carryon",
		Version:  "1.0.0",
		Audience: modv1.ModAudience_MOD_AUDIENCE_REQUIRED,
		Note:     "  нужен всем  ",
	})
	if err != nil {
		t.Fatalf("AddMod: %v", err)
	}

	if len(repo.order) != 0 {
		t.Errorf("adding a mod must shift, not reorder: %v", repo.order)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created %d mods, want 1", len(repo.created))
	}

	added := repo.created[0]
	if added.Position != 0 {
		t.Errorf("position = %d, want the mod first", added.Position)
	}
	if added.Note != "нужен всем" {
		t.Errorf("note = %q, want the trimmed text", added.Note)
	}
	if added.Release == nil || added.Release.Version != "1.0.0" {
		t.Fatalf("release = %v, want the chosen version", added.Release)
	}
	if added.Release.FileSize != 659496 {
		t.Errorf("file_size = %d, want the HEAD answer", added.Release.FileSize)
	}
	// 1.0.0 is chosen while ModDB lists 2.0.0 as the newest.
	if added.LatestVersion != "2.0.0" {
		t.Errorf("latest_version = %q, want 2.0.0", added.LatestVersion)
	}
	if got.GetPosition() != 0 {
		t.Errorf("answer position = %d, want 0", got.GetPosition())
	}
	if repo.mods[0].ModId != "carryon" {
		t.Errorf("first stored mod = %q, want the new one", repo.mods[0].ModId)
	}
}

func TestAddModLatestVersionIsEmptyWhenItIsTheNewest(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(t, repo, &fakeModDB{mod: modDBFixture(), size: 10})

	got, err := svc.AddMod(context.Background(), &modv1.AddModRequest{
		Query:    "carryon",
		Version:  "2.0.0",
		Audience: modv1.ModAudience_MOD_AUDIENCE_OPTIONAL,
	})
	if err != nil {
		t.Fatalf("AddMod: %v", err)
	}

	if got.GetLatestVersion() != "" {
		t.Errorf("latest_version = %q, want empty when the chosen version is the newest", got.GetLatestVersion())
	}
}

func TestAddModRefusesWhatTheApiDocuments(t *testing.T) {
	cases := []struct {
		name string
		req  *modv1.AddModRequest
		want error
	}{
		{
			name: "no audience",
			req: &modv1.AddModRequest{
				Query: "carryon", Version: "1.0.0",
				Audience: modv1.ModAudience_MOD_AUDIENCE_UNSPECIFIED,
			},
			want: ierror.ErrAudience,
		},
		{
			name: "a version ModDB does not list",
			req: &modv1.AddModRequest{
				Query: "carryon", Version: "9.9.9",
				Audience: modv1.ModAudience_MOD_AUDIENCE_REQUIRED,
			},
			want: ierror.ErrUnknownVersion,
		},
		{
			name: "a note over the limit",
			req: &modv1.AddModRequest{
				Query: "carryon", Version: "1.0.0",
				Audience: modv1.ModAudience_MOD_AUDIENCE_REQUIRED,
				Note:     strings.Repeat("я", model.MaxNoteRunes+1),
			},
			want: ierror.ErrNote,
		},
		{
			name: "a query that is not ModDB",
			req: &modv1.AddModRequest{
				Query: "https://example.com/carryon", Version: "1.0.0",
				Audience: modv1.ModAudience_MOD_AUDIENCE_REQUIRED,
			},
			want: ierror.ErrBadQuery,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := newService(t, repo, &fakeModDB{mod: modDBFixture()})

			_, err := svc.AddMod(context.Background(), tc.req)

			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if len(repo.created) != 0 {
				t.Errorf("a refused mod was stored: %v", repo.created)
			}
		})
	}
}

func TestAddModReportsModDBNotAnswering(t *testing.T) {
	repo := &fakeRepo{}
	client := &fakeModDB{err: moddb.ErrUnavailable}
	svc := newService(t, repo, client)

	_, err := svc.AddMod(context.Background(), &modv1.AddModRequest{
		Query: "carryon", Version: "1.0.0",
		Audience: modv1.ModAudience_MOD_AUDIENCE_REQUIRED,
	})

	if !errors.Is(err, ierror.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestUpdateModChangesOnlyWhatWasAsked(t *testing.T) {
	repo := &fakeRepo{mods: []model.Mod{storedMod("carryon", model.AudienceRequired, "1.0.0")}}
	svc := newService(t, repo, &fakeModDB{mod: modDBFixture(), size: 42})

	note := "только клиент"
	audience := modv1.ModAudience_MOD_AUDIENCE_OPTIONAL

	got, err := svc.UpdateMod(context.Background(), &modv1.UpdateModRequest{
		Id:       "carryon",
		Audience: &audience,
		Note:     &note,
	})
	if err != nil {
		t.Fatalf("UpdateMod: %v", err)
	}

	if got.GetAudience() != modv1.ModAudience_MOD_AUDIENCE_OPTIONAL || got.GetNote() != note {
		t.Errorf("mod = %v / %q", got.GetAudience(), got.GetNote())
	}
	if got.GetRelease().GetVersion() != "1.0.0" {
		t.Errorf("version = %q, want it kept", got.GetRelease().GetVersion())
	}
}

func TestUpdateModMovesToAKnownVersionAndRefusesAnUnknownOne(t *testing.T) {
	repo := &fakeRepo{mods: []model.Mod{storedMod("carryon", model.AudienceRequired, "1.0.0")}}
	svc := newService(t, repo, &fakeModDB{mod: modDBFixture(), size: 77})

	version := "2.0.0"
	got, err := svc.UpdateMod(context.Background(), &modv1.UpdateModRequest{Id: "carryon", Version: &version})
	if err != nil {
		t.Fatalf("UpdateMod: %v", err)
	}

	if got.GetRelease().GetVersion() != "2.0.0" || got.GetRelease().GetFileSize() != 77 {
		t.Errorf("release = %v, want 2.0.0 measured again", got.GetRelease())
	}
	if got.GetLatestVersion() != "" {
		t.Errorf("latest_version = %q, want empty now that the newest is chosen", got.GetLatestVersion())
	}

	unknown := "9.9.9"
	if _, err := svc.UpdateMod(context.Background(), &modv1.UpdateModRequest{Id: "carryon", Version: &unknown}); !errors.Is(err, ierror.ErrUnknownVersion) {
		t.Fatalf("err = %v, want ErrUnknownVersion", err)
	}
}

func TestDeleteModRefusesAnUnknownMod(t *testing.T) {
	svc := newService(t, &fakeRepo{}, &fakeModDB{mod: modDBFixture()})

	if _, err := svc.DeleteMod(context.Background(), &modv1.DeleteModRequest{Id: "nothing"}); !errors.Is(err, ierror.ErrNotInList) {
		t.Fatalf("err = %v, want ErrNotInList", err)
	}
}

func TestReorderModsTakesOnlyAPermutation(t *testing.T) {
	repo := &fakeRepo{mods: []model.Mod{
		storedMod("first", model.AudienceRequired, "1.0.0"),
		storedMod("second", model.AudienceOptional, "1.0.0"),
	}}
	svc := newService(t, repo, &fakeModDB{mod: modDBFixture()})

	got, err := svc.ReorderMods(context.Background(), &modv1.ReorderModsRequest{Ids: []string{"second", "first"}})
	if err != nil {
		t.Fatalf("ReorderMods: %v", err)
	}

	if len(got.GetMods()) != 2 || got.GetMods()[0].GetId() != "second" {
		t.Fatalf("answer order = %v, want the new one", got.GetMods())
	}
	// The stored documents carry the new positions; the slice itself is not
	// reordered, because the database orders by position.
	for _, mod := range repo.mods {
		want := 1
		if mod.ModId == "second" {
			want = 0
		}
		if mod.Position != want {
			t.Errorf("%s position = %d, want %d", mod.ModId, mod.Position, want)
		}
	}
	if len(got.GetMods()[0].GetReleases()) == 0 {
		t.Error("the staff's answer carries the releases")
	}

	cases := map[string][]string{
		"a missing id": {"first"},
		"an extra id":  {"first", "second", "third"},
		"a repeat":     {"first", "first"},
		"nothing":      nil,
	}

	for name, ids := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.ReorderMods(context.Background(), &modv1.ReorderModsRequest{Ids: ids}); !errors.Is(err, ierror.ErrOrder) {
				t.Fatalf("err = %v, want ErrOrder", err)
			}
		})
	}
}

func TestRefreshKeepsWhatIsStoredWhenModDBFails(t *testing.T) {
	stored := storedMod("carryon", model.AudienceRequired, "1.0.0")
	stored.SetNote("старая заметка")
	repo := &fakeRepo{mods: []model.Mod{stored}}

	svc := newService(t, repo, &fakeModDB{err: moddb.ErrUnavailable})

	got, err := svc.RefreshMods(context.Background(), &modv1.RefreshModsRequest{})
	if err != nil {
		t.Fatalf("a mod ModDB cannot answer for must not fail the call: %v", err)
	}

	if len(got.GetMods()) != 1 {
		t.Fatalf("mods = %d, want the stored one", len(got.GetMods()))
	}
	if got.GetMods()[0].GetNote() != "старая заметка" {
		t.Errorf("note = %q, want it kept", got.GetMods()[0].GetNote())
	}
	if got.GetMods()[0].GetRelease().GetVersion() != "1.0.0" {
		t.Errorf("version = %q, want it kept", got.GetMods()[0].GetRelease().GetVersion())
	}
}

func TestRefreshUpdatesTheSnapshotAndKeepsTheChosenVersion(t *testing.T) {
	repo := &fakeRepo{mods: []model.Mod{storedMod("carryon", model.AudienceRequired, "1.0.0")}}
	svc := newService(t, repo, &fakeModDB{mod: modDBFixture(), size: 5})

	got, err := svc.RefreshMods(context.Background(), &modv1.RefreshModsRequest{})
	if err != nil {
		t.Fatalf("RefreshMods: %v", err)
	}

	mod := got.GetMods()[0]
	if mod.GetName() != "Carry On" || mod.GetAuthor() != "NerdScurvy" {
		t.Errorf("snapshot = %s / %s", mod.GetName(), mod.GetAuthor())
	}
	if mod.GetRelease().GetVersion() != "1.0.0" {
		t.Errorf("version = %q, want the chosen one kept", mod.GetRelease().GetVersion())
	}
	if mod.GetRelease().GetFileSize() != 5 {
		t.Errorf("file_size = %d, want the fresh HEAD answer", mod.GetRelease().GetFileSize())
	}
	if len(mod.GetReleases()) != 2 {
		t.Errorf("releases = %d, want the fresh snapshot", len(mod.GetReleases()))
	}
}

func TestListModsHidesHiddenAndDescribesTheArchive(t *testing.T) {
	required := storedMod("required", model.AudienceRequired, "1.0.0")
	optional := storedMod("optional", model.AudienceOptional, "1.0.0")
	hidden := storedMod("hidden", model.AudienceHidden, "1.0.0")

	repo := &fakeRepo{mods: []model.Mod{required, optional, hidden}}
	svc := newService(t, repo, &fakeModDB{mod: modDBFixture()})

	got, err := svc.ListMods(context.Background(), &modv1.ListModsRequest{})
	if err != nil {
		t.Fatalf("ListMods: %v", err)
	}

	if len(got.GetMods()) != 2 {
		t.Fatalf("mods = %d, want the required and the optional one", len(got.GetMods()))
	}
	for _, mod := range got.GetMods() {
		if mod.GetId() == "hidden" {
			t.Error("a HIDDEN mod reached the public list")
		}
		if len(mod.GetReleases()) != 0 {
			t.Error("the public list must not carry the release history")
		}
	}

	archive := got.GetArchive()
	if archive == nil {
		t.Fatal("archive is unset, want the required mods described")
	}
	if archive.GetUrl() != "/v1/mods/archive.zip" || archive.GetCount() != 1 {
		t.Errorf("archive = %s / %d", archive.GetUrl(), archive.GetCount())
	}
	if archive.GetSize() != 1024 {
		t.Errorf("archive size = %d, want the total of the required mods", archive.GetSize())
	}
}

func TestListModsWithoutRequiredModsHasNoArchive(t *testing.T) {
	repo := &fakeRepo{mods: []model.Mod{storedMod("optional", model.AudienceOptional, "1.0.0")}}
	svc := newService(t, repo, &fakeModDB{mod: modDBFixture()})

	got, err := svc.ListMods(context.Background(), &modv1.ListModsRequest{})
	if err != nil {
		t.Fatalf("ListMods: %v", err)
	}

	if got.GetArchive() != nil {
		t.Errorf("archive = %v, want unset", got.GetArchive())
	}
}

func TestScopeGatesEveryManagingMethod(t *testing.T) {
	svc := newService(t, &fakeRepo{}, &fakeModDB{mod: modDBFixture()})
	scope := svc.Scope()

	managing := []string{
		"ListModsAdmin", "LookupMod", "AddMod", "UpdateMod", "DeleteMod", "ReorderMods", "RefreshMods",
	}

	for _, method := range managing {
		got, ok := scope[interceptor.Method("/mod.v1.ModService/"+method)]
		if !ok {
			t.Errorf("%s is not classified for auth", method)
			continue
		}
		if got != interceptor.Scope("news:create") {
			t.Errorf("%s scope = %q, want news:create", method, got)
		}
	}

	if _, ok := scope["/mod.v1.ModService/ListMods"]; ok {
		t.Error("ListMods is public and must not be in the scope table")
	}
}
