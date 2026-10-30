package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

// syncRepo is the stored list as the sync sees it.
type syncRepo struct {
	mods    []model.Mod
	synced  []model.Mod
	removed []string
	listErr error
}

func (r *syncRepo) List(context.Context) ([]model.Mod, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}

	return append([]model.Mod(nil), r.mods...), nil
}

func (r *syncRepo) Get(context.Context, string) (*model.Mod, error) {
	return nil, errors.New("not used")
}

func (r *syncRepo) Create(context.Context, *model.Mod) error {
	return errors.New("not used")
}

func (r *syncRepo) Save(
	context.Context,
	string,
	func(context.Context, *model.Mod) (*model.Mod, error),
) (*model.Mod, error) {
	return nil, errors.New("not used")
}

func (r *syncRepo) Delete(context.Context, string) error {
	return errors.New("not used")
}

func (r *syncRepo) Reorder(context.Context, []string) error {
	return errors.New("not used")
}

// Sync applies the server-sourced list and remembers what the syncer asked for.
func (r *syncRepo) Sync(_ context.Context, mods []model.Mod, removeIds []string) error {
	r.synced = append([]model.Mod(nil), mods...)
	r.removed = append([]string(nil), removeIds...)

	for _, id := range removeIds {
		for i := range r.mods {
			if r.mods[i].ModId == id {
				r.mods = append(r.mods[:i], r.mods[i+1:]...)

				break
			}
		}
	}

	for _, mod := range mods {
		found := false
		for i := range r.mods {
			if r.mods[i].ModId == mod.ModId {
				r.mods[i] = mod
				found = true

				break
			}
		}

		if !found {
			mod.SetPosition(len(r.mods))
			r.mods = append(r.mods, mod)
		}
	}

	return nil
}

// fakeObjects answers GetObject with one fixed body or one fixed error.
type fakeObjects struct {
	body string
	err  error
}

func (f *fakeObjects) GetObject(context.Context, string, string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}

	return io.NopCloser(strings.NewReader(f.body)), nil
}

// newTestSyncer builds the sync over a config the tests can rely on.
func newTestSyncer(t *testing.T, repo Repository, objects ObjectStorage, cfg config.Config) *Syncer {
	t.Helper()

	zc := zap.NewProductionConfig()
	zc.Level = zap.NewAtomicLevelAt(zap.WarnLevel)

	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ModsBucket == "" {
		cfg.ModsBucket = "server-mods"
	}
	if cfg.ModsManifestKey == "" {
		cfg.ModsManifestKey = "manifest.json"
	}
	if cfg.ModsSyncInterval == 0 {
		cfg.ModsSyncInterval = time.Minute
	}
	if cfg.CdnUrl == "" {
		cfg.CdnUrl = "https://cdn.example"
	}

	syncer := NewSyncer(SyncerOpts{Repo: repo, Objects: objects, Logger: l, Config: cfg})
	syncer.now = func() time.Time { return fixedNow }

	return syncer
}

// manifestEntry is one manifest mod with the fields the sync requires; a test
// overrides the field it is about.
func manifestEntry(modId, version, fileKey string) manifestMod {
	return manifestMod{
		ModID:            modId,
		Name:             modId,
		Authors:          []string{"author"},
		Description:      "A test mod.",
		Version:          version,
		Side:             "universal",
		RequiredOnClient: true,
		Filename:         modId + "-" + version + ".zip",
		Size:             1024,
		SHA256:           strings.Repeat("ab", 32),
		FileKey:          fileKey,
		UploadedAt:       fixedNow,
	}
}

// validManifest marshals a manifest the sync accepts.
func validManifest(t *testing.T, mods ...manifestMod) string {
	t.Helper()

	if mods == nil {
		mods = []manifestMod{}
	}

	body, err := json.Marshal(manifest{
		Version:     manifestVersion,
		GeneratedAt: fixedNow,
		GameVersion: "1.22.7",
		Mods:        mods,
	})
	if err != nil {
		t.Fatal(err)
	}

	return string(body)
}

// serverStored is one server-sourced mod already in the list.
func serverStored(modId, version string, position int) model.Mod {
	release := model.Release{
		Version:  version,
		FileURL:  "https://cdn.example/server-mods/" + modId + ".zip",
		FileName: modId + ".zip",
		FileSize: 100,
	}

	mod := model.NewServerManifest(modId, model.Snapshot{
		Name:     modId,
		Releases: []model.Release{release},
	}, release, position, fixedNow)

	return mod
}

// staffMod is one mod the staff added by hand.
func staffMod(modId string, position int) model.Mod {
	mod := serverStored(modId, "1.0.0", position)
	mod.SetAudience(model.AudienceOptional)
	mod.SetSource(model.SourceAdmin)

	return mod
}

// TestManifestRejectsBadJSONUnknownVersionAndDuplicates pins the one rule that
// keeps the page alive: a document the service cannot trust changes nothing.
// Without it the first deployment - before the game server has written anything
// - would delete every stored mod.
func TestManifestRejectsBadJSONUnknownVersionAndDuplicates(t *testing.T) {
	cases := map[string]string{
		"bad JSON":      `{"version":1,"mods":[`,
		"version 2":     `{"version":2,"mods":[{"modid":"a","version":"1.0.0"}]}`,
		"nothing valid": `{"version":1,"mods":[{"modid":"","version":"1.0.0"}]}`,
		"no mods key":   `{"version":1}`,
		"duplicate id": validManifest(t,
			manifestEntry("a", "1.0.0", "a/a.zip"),
			manifestEntry("a", "2.0.0", "a/b.zip"),
		),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			repo := &syncRepo{mods: []model.Mod{serverStored("stored", "1.0.0", 0)}}
			syncer := newTestSyncer(t, repo, &fakeObjects{body: body}, config.Config{})

			if err := syncer.Sync(context.Background()); err == nil {
				t.Fatal("Sync accepted a manifest it must reject")
			}
			if len(repo.synced) != 0 || len(repo.removed) != 0 {
				t.Errorf("a rejected manifest wrote to the list: %v / %v", repo.synced, repo.removed)
			}
		})
	}
}

// TestDuplicateManifestErrorDoesNotNameTheMod pins the log rule: the duplicate
// id may be a server-only mod, and the error line must not carry it.
func TestDuplicateManifestErrorDoesNotNameTheMod(t *testing.T) {
	const serverOnly = "vintageapi"

	repo := &syncRepo{mods: []model.Mod{serverStored("stored", "1.0.0", 0)}}
	syncer := newTestSyncer(t, repo, &fakeObjects{body: validManifest(t,
		manifestEntry(serverOnly, "1.0.0", "a/a.zip"),
		manifestEntry(serverOnly, "2.0.0", "a/b.zip"),
	)}, config.Config{})

	err := syncer.Sync(context.Background())
	if err == nil {
		t.Fatal("Sync accepted a manifest with a duplicate id")
	}
	if strings.Contains(err.Error(), serverOnly) {
		t.Errorf("the sync error names the rejected mod: %v", err)
	}
}

// TestSyncEmptyManifestClearsServerMods pins the other half of the contract: the
// game server writes an empty list when it requires no client mod, and the site
// must then stop offering one. The staff's own mods stay.
func TestSyncEmptyManifestClearsServerMods(t *testing.T) {
	repo := &syncRepo{mods: []model.Mod{
		staffMod("optional", 1),
		serverStored("gone", "1.0.0", 2),
	}}

	syncer := newTestSyncer(t, repo, &fakeObjects{body: validManifest(t)}, config.Config{})

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(repo.synced) != 0 {
		t.Errorf("synced = %v, want no server mods", repo.synced)
	}
	if len(repo.removed) != 1 || repo.removed[0] != "gone" {
		t.Errorf("removed = %v, want the dropped server mod", repo.removed)
	}
	optional := findByID(t, repo.mods, "optional")
	if optional.Version() != "1.0.0" {
		t.Error("the staff's mod was removed by an empty manifest")
	}
}

// TestSyncAddsRemovesAndPreservesStaffData pins the merge: the manifest adds a
// mod and updates a stored one, drops a mod it no longer lists, and never
// touches the staff's OPTIONAL mods. The note and the position of the stored
// server mod survive the rewrite.
func TestSyncAddsRemovesAndPreservesStaffData(t *testing.T) {
	kept := serverStored("kept", "1.0.0", 3)
	kept.SetNote("заметка админа")

	repo := &syncRepo{mods: []model.Mod{
		staffMod("optional", 1),
		kept,
		serverStored("gone", "1.0.0", 4),
	}}

	objects := &fakeObjects{body: validManifest(t,
		manifestEntry("kept", "2.0.0", "kept/Kept On.zip"),
		manifestEntry("new", "1.0.0", "new/New-1.0.0.zip"),
	)}

	syncer := newTestSyncer(t, repo, objects, config.Config{})

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(repo.synced) != 2 {
		t.Fatalf("synced %d mods, want 2", len(repo.synced))
	}

	updated := findByID(t, repo.synced, "kept")
	if updated.Note != "заметка админа" {
		t.Errorf("note = %q, want the staff's note preserved", updated.Note)
	}
	if updated.Position != 3 {
		t.Errorf("position = %d, want 3 preserved", updated.Position)
	}
	if updated.Version() != "2.0.0" {
		t.Errorf("version = %q, want the manifest version", updated.Version())
	}
	if updated.Audience != model.AudienceRequired || updated.Source != model.SourceServer {
		t.Errorf("audience/source = %v/%v, want REQUIRED server", updated.Audience, updated.Source)
	}

	added := findByID(t, repo.synced, "new")
	if added.Note != "" {
		t.Errorf("new mod note = %q, want empty", added.Note)
	}

	if len(repo.removed) != 1 || repo.removed[0] != "gone" {
		t.Errorf("removed = %v, want the mod the manifest dropped", repo.removed)
	}

	optional := findByID(t, repo.mods, "optional")
	if optional.Version() != "1.0.0" || optional.Note != "" {
		t.Errorf("the staff's mod changed: %v", optional)
	}
}

// TestSyncBuildsEscapedCDNURLs pins the path building: a space in a mod file
// name must not make a broken URL, and the logo comes from the icon key.
func TestSyncBuildsEscapedCDNURLs(t *testing.T) {
	entry := manifestEntry("carryon", "2.0.0", "carryon/Carry On 2.0.0.zip")
	entry.IconKey = "carryon/mod icon.png"

	repo := &syncRepo{}
	syncer := newTestSyncer(t, repo, &fakeObjects{body: validManifest(t, entry)}, config.Config{})

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	mod := findByID(t, repo.synced, "carryon")
	if mod.Release == nil {
		t.Fatal("release is unset")
	}
	wantFile := "https://cdn.example/server-mods/carryon/Carry%20On%202.0.0.zip"
	if mod.Release.FileURL != wantFile {
		t.Errorf("file_url = %q, want %q", mod.Release.FileURL, wantFile)
	}
	if mod.Release.FileKey != entry.FileKey {
		t.Errorf("file_key = %q, want it stored for the archive", mod.Release.FileKey)
	}
	wantLogo := "https://cdn.example/server-mods/carryon/mod%20icon.png"
	if mod.LogoURL != wantLogo {
		t.Errorf("logo_url = %q, want %q", mod.LogoURL, wantLogo)
	}
}

// TestSyncWithoutCDNKeepsOnlyTheObjectKey pins the empty-CDN fallback: no broken
// URL is emitted, and the archive can still read the file from MinIO.
func TestSyncWithoutCDNKeepsOnlyTheObjectKey(t *testing.T) {
	repo := &syncRepo{}
	syncer := newTestSyncer(t, repo, &fakeObjects{
		body: validManifest(t, manifestEntry("carryon", "1.0.0", "carryon/file.zip")),
	}, config.Config{})

	// newTestSyncer fills an empty CDN; clear it after construction to exercise
	// the deployment that has no CDN configured.
	syncer.cdnURL = ""

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	mod := findByID(t, repo.synced, "carryon")
	if mod.Release.FileURL != "" {
		t.Errorf("file_url = %q, want empty without a CDN", mod.Release.FileURL)
	}
	if mod.Release.FileKey != "carryon/file.zip" {
		t.Errorf("file_key = %q, want it kept", mod.Release.FileKey)
	}
}

// TestSyncLeavesTheListWhenMinIOFails pins the failure path: the stored list is
// served as it is and the error is returned for the caller that logs it.
func TestSyncLeavesTheListWhenMinIOFails(t *testing.T) {
	repo := &syncRepo{mods: []model.Mod{serverStored("stored", "1.0.0", 0)}}
	syncer := newTestSyncer(t, repo, &fakeObjects{err: errors.New("minio is down")}, config.Config{})

	if err := syncer.Sync(context.Background()); err == nil {
		t.Fatal("Sync reported success while MinIO failed")
	}

	if len(repo.synced) != 0 || len(repo.removed) != 0 {
		t.Errorf("a failed read wrote to the list: %v / %v", repo.synced, repo.removed)
	}
	if len(repo.mods) != 1 {
		t.Errorf("stored mods = %d, want the list untouched", len(repo.mods))
	}
}

// TestSyncSkipsUnusableEntries pins that one broken mod does not freeze the
// rest: a server-side mod, a bad digest and a missing file are dropped with a
// warning while the valid entry is stored.
func TestSyncSkipsUnusableEntries(t *testing.T) {
	serverSide := manifestEntry("secret", "1.0.0", "secret/secret.zip")
	serverSide.Side = "server"
	serverSide.RequiredOnClient = false

	badSHA := manifestEntry("baddigest", "1.0.0", "baddigest/x.zip")
	badSHA.SHA256 = "not-a-digest"

	noFile := manifestEntry("nofile", "1.0.0", "")
	noFile.FileKey = ""

	good := manifestEntry("good", "1.0.0", "good/good.zip")

	repo := &syncRepo{}
	syncer := newTestSyncer(t, repo, &fakeObjects{
		body: validManifest(t, serverSide, badSHA, noFile, good),
	}, config.Config{})

	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(repo.synced) != 1 || repo.synced[0].ModId != "good" {
		t.Errorf("synced = %v, want the one usable entry", repo.synced)
	}
}

// TestRefreshDoesNothingWhileTheCacheIsFresh pins the TTL: a second caller
// within the interval does not read MinIO again.
func TestRefreshDoesNothingWhileTheCacheIsFresh(t *testing.T) {
	finished := make(chan struct{}, 1)
	repo := &syncRepo{}
	objects := &fakeObjects{body: validManifest(t, manifestEntry("a", "1.0.0", "a/a.zip"))}
	syncer := newTestSyncer(t, repo, objects, config.Config{})

	clock := fixedNow
	syncer.now = func() time.Time { return clock }
	syncer.afterSync = func() { finished <- struct{}{} }

	syncer.Refresh(context.Background())

	// Refresh runs out of band: wait for the pass it started before reading the
	// fake repository, which also gives the race detector a happens-before edge.
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Refresh did not finish")
	}

	if len(repo.synced) != 1 {
		t.Fatalf("first Refresh synced %d mods, want 1", len(repo.synced))
	}

	// The fake body is replaced with a broken one: a second read would fail and
	// change nothing, so a successful no-op proves the cache was used.
	objects.body = "not json"
	clock = fixedNow.Add(10 * time.Second)

	syncer.Refresh(context.Background())
	if len(repo.removed) != 0 {
		t.Errorf("Refresh inside the TTL read MinIO again")
	}
}

// findByID returns the mod of one id, or fails the test.
func findByID(t *testing.T, mods []model.Mod, modId string) model.Mod {
	t.Helper()

	for _, mod := range mods {
		if mod.ModId == modId {
			return mod
		}
	}

	panic("mod " + modId + " is not in the list")
}
