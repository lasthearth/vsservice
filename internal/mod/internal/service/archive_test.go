package service

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/zap"
)

// fixedMods answers ListRequired with whatever the test put in it.
type fixedMods struct{ mods []model.Mod }

func (f *fixedMods) ListRequired(context.Context) ([]model.Mod, error) { return f.mods, nil }

// fileServer serves a body per path; a path missing from bodies answers 500.
func fileServer(t *testing.T, bodies map[string]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.Error(w, "gone", http.StatusInternalServerError)

			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return server
}

// servedMod is a required mod whose file the test server holds.
func servedMod(server *httptest.Server, modId, version string, size int64) model.Mod {
	release := model.Release{
		Version:  version,
		FileURL:  server.URL + "/" + modId + "-" + version + ".zip",
		FileName: modId + "-" + version + ".zip",
		FileSize: size,
	}

	return model.Build(modId, model.Snapshot{Name: modId, Releases: []model.Release{release}},
		model.AudienceRequired, "", &release, fixedNow)
}

// newTestArchive builds an Archive over dir with the given cap. cdn is the
// configured CDN base; a test that serves files from httptest passes its URL.
func newTestArchive(t *testing.T, mods *fixedMods, dir string, maxBytes int64, cdn string) *Archive {
	t.Helper()

	zc := zap.NewDevelopmentConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	archive, err := NewArchive(mods, nil, l, config.Config{
		ModsArchiveDir:      dir,
		ModsArchiveMaxBytes: maxBytes,
		CdnUrl:              cdn,
	})
	if err != nil {
		t.Fatal(err)
	}

	return archive
}

// get asks the archive handler for the archive.
func get(archive *Archive) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	archive.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/mods/archive.zip", nil))

	return recorder
}

// files lists the names in dir.
func files(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	return names
}

func TestArchiveSweepsOwnFilesOnStartAndLeavesOthers(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"archive-old.zip", "archive-crashed.zip.part", "someone-else.zip", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	newTestArchive(t, &fixedMods{}, dir, 0, "")

	got := strings.Join(files(t, dir), ",")
	if got != "notes.txt,someone-else.zip" {
		t.Errorf("folder after start = %s, want only the files that are not the archive's", got)
	}
}

func TestArchiveKeepsOneFileAcrossListChanges(t *testing.T) {
	server := fileServer(t, map[string]string{"/a-1.zip": "aaaa", "/a-2.zip": "bbbb", "/b-1.zip": "cc"})
	dir := t.TempDir()
	mods := &fixedMods{mods: []model.Mod{servedMod(server, "a", "1", 4)}}
	archive := newTestArchive(t, mods, dir, 0, server.URL)

	for _, list := range [][]model.Mod{
		{servedMod(server, "a", "1", 4)},
		{servedMod(server, "a", "2", 4)},
		{servedMod(server, "a", "2", 4), servedMod(server, "b", "1", 2)},
	} {
		mods.mods = list
		if code := get(archive).Code; code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if got := files(t, dir); len(got) != 1 || !strings.HasSuffix(got[0], ".zip") {
			t.Fatalf("folder = %v, want exactly one finished archive", got)
		}
	}

	reader, err := zip.OpenReader(filepath.Join(dir, files(t, dir)[0]))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	if len(reader.File) != 2 || reader.File[0].Name != "a-2.zip" || reader.File[1].Name != "b-1.zip" {
		t.Errorf("archive holds %d entries, want a-2.zip and b-1.zip in display order", len(reader.File))
	}
}

func TestArchiveFailedBuildLeavesNothing(t *testing.T) {
	server := fileServer(t, map[string]string{"/a-1.zip": "aaaa"})
	dir := t.TempDir()
	mods := &fixedMods{mods: []model.Mod{servedMod(server, "a", "1", 4), servedMod(server, "missing", "1", 4)}}
	archive := newTestArchive(t, mods, dir, 0, server.URL)

	if code := get(archive).Code; code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", code)
	}
	if got := files(t, dir); len(got) != 0 {
		t.Errorf("folder after a failed build = %v, want empty", got)
	}
}

func TestArchiveOverTheCapIsRefusedBeforeDownloading(t *testing.T) {
	server := fileServer(t, map[string]string{"/a-1.zip": "aaaa"})
	dir := t.TempDir()
	archive := newTestArchive(t, &fixedMods{mods: []model.Mod{servedMod(server, "a", "1", 2048)}}, dir, 1024, server.URL)

	if code := get(archive).Code; code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", code)
	}
	if got := files(t, dir); len(got) != 0 {
		t.Errorf("folder = %v, want empty", got)
	}
}

func TestArchiveUnknownSizeIsCutAtTheCap(t *testing.T) {
	server := fileServer(t, map[string]string{"/a-1.zip": strings.Repeat("x", 4096)})
	dir := t.TempDir()
	archive := newTestArchive(t, &fixedMods{mods: []model.Mod{servedMod(server, "a", "1", 0)}}, dir, 1024, server.URL)

	if code := get(archive).Code; code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", code)
	}
	if got := files(t, dir); len(got) != 0 {
		t.Errorf("folder = %v, want the cut-off .part removed", got)
	}
}

func TestArchiveIsNotOfferedOverTheCap(t *testing.T) {
	mods := []model.Mod{requiredMod("big", "1.0.0", 2048)}

	if got := archiveProto(mods, 1024); got != nil {
		t.Errorf("archive = %v, want unset: the endpoint would refuse it", got)
	}
	if got := archiveProto(mods, 0); got == nil {
		t.Error("archive is unset without a cap")
	}
}

// keyedObjects is the bucket side of the archive: one body per object key.
type keyedObjects struct {
	bodies map[string]string
	calls  int
}

func (f *keyedObjects) GetObject(_ context.Context, _, key string) (io.ReadCloser, error) {
	f.calls++

	body, ok := f.bodies[key]
	if !ok {
		return nil, errors.New("no such object")
	}

	return io.NopCloser(strings.NewReader(body)), nil
}

// newTestArchiveWithObjects builds an Archive that can read from the bucket.
func newTestArchiveWithObjects(
	t *testing.T, mods *fixedMods, objects ObjectStorage, dir, cdn string,
) *Archive {
	t.Helper()

	zc := zap.NewDevelopmentConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	archive, err := NewArchive(mods, objects, l, config.Config{ModsArchiveDir: dir, CdnUrl: cdn})
	if err != nil {
		t.Fatal(err)
	}

	return archive
}

// objectMod is a required mod that names its bucket object and no HTTP file.
func objectMod(modId, version, key string, size int64) model.Mod {
	release := model.Release{
		Version:  version,
		FileName: modId + "-" + version + ".zip",
		FileSize: size,
		FileKey:  key,
	}

	return model.Build(modId, model.Snapshot{Name: modId, Releases: []model.Release{release}},
		model.AudienceRequired, "", &release, fixedNow)
}

// TestArchiveReadsTheBucketWhenNoCDNIsConfigured pins the fallback the site
// depends on when CDN_URL is empty: the manifest still names the object key.
func TestArchiveReadsTheBucketWhenNoCDNIsConfigured(t *testing.T) {
	objects := &keyedObjects{bodies: map[string]string{"a/a-1.zip": "aaaa"}}
	dir := t.TempDir()
	archive := newTestArchiveWithObjects(t, &fixedMods{mods: []model.Mod{objectMod("a", "1", "a/a-1.zip", 4)}}, objects, dir, "")

	if code := get(archive).Code; code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if objects.calls != 1 {
		t.Errorf("bucket reads = %d, want 1", objects.calls)
	}
}

// TestArchiveFallsBackToTheBucketWhenHTTPFails: a CDN that does not serve the
// bucket yet must not make the archive unbuildable while MinIO has the bytes.
func TestArchiveFallsBackToTheBucketWhenHTTPFails(t *testing.T) {
	server := fileServer(t, nil)
	objects := &keyedObjects{bodies: map[string]string{"a/a-1.zip": "aaaa"}}

	release := model.Release{
		Version:  "1",
		FileURL:  server.URL + "/a-1.zip",
		FileName: "a-1.zip",
		FileSize: 4,
		FileKey:  "a/a-1.zip",
	}
	mod := model.Build("a", model.Snapshot{Name: "a", Releases: []model.Release{release}},
		model.AudienceRequired, "", &release, fixedNow)

	dir := t.TempDir()
	archive := newTestArchiveWithObjects(t, &fixedMods{mods: []model.Mod{mod}}, objects, dir, server.URL)

	if code := get(archive).Code; code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after the bucket fallback", code)
	}
	if objects.calls != 1 {
		t.Errorf("bucket reads = %d, want 1", objects.calls)
	}
}

// TestArchiveRefusesAFileFromAnotherHost pins the allow-list: a release URL
// that is neither the CDN nor ModDB's own file host is not fetched, so a future
// writer of file_url cannot turn the archive into an SSRF primitive.
func TestArchiveRefusesAFileFromAnotherHost(t *testing.T) {
	server := fileServer(t, map[string]string{"/a-1.zip": "aaaa"})
	release := model.Release{
		Version:  "1",
		FileURL:  "https://evil.example/a-1.zip",
		FileName: "a-1.zip",
		FileSize: 4,
	}
	mod := model.Build("a", model.Snapshot{Name: "a", Releases: []model.Release{release}},
		model.AudienceRequired, "", &release, fixedNow)

	dir := t.TempDir()
	archive := newTestArchive(t, &fixedMods{mods: []model.Mod{mod}}, dir, 0, server.URL)

	if code := get(archive).Code; code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 for a foreign host", code)
	}
}
