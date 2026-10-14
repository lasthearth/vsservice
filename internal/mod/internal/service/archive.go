package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/moddb"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const (
	// archiveFileName is what the browser saves the archive as.
	archiveFileName = "lasthearth-mods.zip"
	// archiveContentType is the type of the built file.
	archiveContentType = "application/zip"
	// archiveMaxAge is how long a client may reuse the archive. It is the
	// window in which a mod added or re-versioned after the download is not
	// seen, which is why it is minutes and not hours.
	archiveMaxAge = 300
	// archiveBuildTimeout bounds one build. It is not the caller's deadline:
	// the first player to ask starts a build every other waiting player shares,
	// so one player closing the tab must not cancel it for the rest.
	archiveBuildTimeout = 5 * time.Minute

	// archivePrefix, archiveSuffix and archivePartSuffix name the files in the
	// archive folder: archive-<key>.zip, and archive-<key>.zip.part while it is
	// written. Nothing else in the folder is ever removed.
	archivePrefix     = "archive-"
	archiveSuffix     = ".zip"
	archivePartSuffix = ".part"
)

var (
	// errArchiveTooLarge is a set of required mods bigger than the cap.
	errArchiveTooLarge = errors.New("the required mods exceed the archive cap")
	// errArchiveNoSpace is a disk without room for the build.
	errArchiveNoSpace = errors.New("not enough free disk space for the archive")
	// errArchiveUnbounded is a build that nothing would cap.
	errArchiveUnbounded = errors.New("the archive has no size bound")

	// maxEntryNameBytes caps one entry's name. A longer one makes CreateHeader
	// fail the whole build, and no extractor or filesystem would have used it.
	maxEntryNameBytes = 255
)

// Archive packs the required mods into one zip and serves it over plain HTTP.
//
// The archive is built once per set of (id, version) pairs and kept as a file
// in a folder the service owns. Rebuilding it per request would pull every mod
// out of ModDB again for every player, and the same list is what all of them
// download.
//
// The folder never holds more than one finished archive and one being written:
// a build writes archive-<key>.zip.part and renames it when it is complete,
// every other archive file is removed after a successful build, and the folder
// is swept on start, so neither a restart nor a crash mid-build leaves anything
// behind. Only files named like the archive are ever removed, so pointing
// MODS_ARCHIVE_DIR at a shared folder cannot cost anyone else's files.
type Archive struct {
	mods  RequiredMods
	moddb *moddb.Client
	log   logger.Logger

	// dir holds the archive files.
	dir string
	// maxBytes caps the archive; 0 means no cap.
	maxBytes int64

	mu    sync.Mutex
	key   string
	path  string
	group singleflight.Group
}

// NewArchive prepares the archive folder and sweeps what an earlier run left.
func NewArchive(mods RequiredMods, client *moddb.Client, log logger.Logger, cfg config.Config) (*Archive, error) {
	if cfg.ModsArchiveMaxBytes < 0 {
		// Every consumer reads the cap as "no cap" when it is not positive, and
		// a negative one also skips the free-space check. Refusing to start is
		// the only answer that does not silently remove both bounds; 0 is how an
		// operator asks for no cap.
		return nil, fmt.Errorf(
			"MODS_ARCHIVE_MAX_BYTES is %d: a negative cap bounds nothing, use 0 for no cap",
			cfg.ModsArchiveMaxBytes)
	}

	dir := cfg.ModsArchiveDir
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "vsservice-mods")
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create the archive directory: %w", err)
	}

	archive := &Archive{
		mods:     mods,
		moddb:    client,
		log:      log.WithComponent("archive"),
		dir:      dir,
		maxBytes: cfg.ModsArchiveMaxBytes,
	}

	// A previous run may have stopped mid-build or kept an archive for a list
	// that has changed since; either is rebuilt on the first request.
	archive.sweep("")

	return archive, nil
}

// ServeHTTP answers GET /v1/mods/archive.zip.
func (a *Archive) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	required, err := a.mods.ListRequired(r.Context())
	if err != nil {
		a.log.Error("failed to list the required mods", zap.Error(err))
		http.Error(w, "could not read the mod list", http.StatusInternalServerError)

		return
	}

	// No required mods is no archive: an empty zip would look like a download
	// that worked.
	if len(required) == 0 {
		http.Error(w, "the server has no required mods", http.StatusNotFound)

		return
	}

	path, err := a.file(r.Context(), archiveKey(required), required)
	if err != nil {
		// Nothing is cached on a failure: a partial archive is worse than an
		// error, because a player would install it and then fail to join.
		switch {
		case errors.Is(err, errArchiveTooLarge):
			a.log.Warn("the required mods exceed the archive cap", zap.Int64("max_bytes", a.maxBytes))
			http.Error(w, "the required mods are too large for one archive", http.StatusRequestEntityTooLarge)
		case errors.Is(err, errArchiveNoSpace):
			a.log.Warn("not enough free disk space to build the archive", zap.Error(err))
			http.Error(w, "the archive cannot be built right now", http.StatusServiceUnavailable)
		case errors.Is(err, errArchiveUnbounded):
			a.log.Error("the archive has no size bound", zap.Error(err))
			http.Error(w, "the archive is not configured", http.StatusInternalServerError)
		default:
			a.log.Error("failed to build the archive", zap.Error(err))
			http.Error(w, "could not build the archive", http.StatusBadGateway)
		}

		return
	}

	//nolint:gosec // G304: the path is a file this service built in its own
	// archive folder, never one a caller names.
	file, err := os.Open(path)
	if err != nil {
		// The file is gone: drop it from the cache, so the next request rebuilds
		// rather than answering 500 until the mod list happens to change.
		a.forget(path)
		a.log.Error("failed to open the built archive", zap.Error(err))
		http.Error(w, "could not read the archive", http.StatusInternalServerError)

		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		a.log.Error("failed to stat the built archive", zap.Error(err))
		http.Error(w, "could not read the archive", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", archiveContentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+archiveFileName+"\"")
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", archiveMaxAge))

	// ServeContent answers a range request and a HEAD with what the archive
	// already is, so a client that resumes a download gets the rest of it.
	http.ServeContent(w, r, archiveFileName, info.ModTime(), file)
}

// file returns the built archive for key, building it when the cached one
// belongs to another set of versions.
//
// Concurrent requests share one build: without that, the moment a new mod is
// announced every player clicking "download everything" would start pulling
// every mod out of ModDB at once.
func (a *Archive) file(ctx context.Context, key string, mods []model.Mod) (string, error) {
	if path, ok := a.cached(key); ok {
		return path, nil
	}

	// DoChan rather than Do: a build can take minutes, and a caller who hangs up
	// should stop holding a goroutine and a connection for it. The build was
	// started without this caller's cancellation, so everyone else waiting on the
	// same key still gets it.
	wait := a.group.DoChan(key, func() (any, error) {
		// Another request may have finished this very key while this one waited
		// for its turn.
		if path, ok := a.cached(key); ok {
			return path, nil
		}

		buildCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), archiveBuildTimeout)
		defer cancel()

		path, err := a.build(buildCtx, key, mods)
		if err != nil {
			return "", err
		}

		a.mu.Lock()
		a.key, a.path = key, path
		a.mu.Unlock()

		// Whatever else is finished in the folder is for versions nobody is
		// served any more. A player still downloading an older archive keeps
		// reading it: removing an open file only unlinks its name.
		a.sweep(path)

		return path, nil
	})

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-wait:
		if result.Err != nil {
			return "", result.Err
		}

		path, _ := result.Val.(string)

		return path, nil
	}
}

// cached is the built archive for key, when the file is still on disk. The
// folder is not private to this process - a second replica starting up sweeps
// it, and a cleaner for the temporary folder empties it - so a remembered name
// is checked rather than trusted.
func (a *Archive) cached(key string) (string, bool) {
	a.mu.Lock()
	path, hit := a.path, a.key == key && a.path != ""
	a.mu.Unlock()

	if !hit {
		return "", false
	}
	if _, err := os.Stat(path); err != nil {
		a.forget(path)

		return "", false
	}

	return path, true
}

// forget drops a cached path, so the next request rebuilds instead of answering
// from a file that is gone.
func (a *Archive) forget(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.path == path {
		a.key, a.path = "", ""
	}
}

// finishedBytes is the size of the archive being served now: the room a build
// has to leave for the players still downloading it.
func (a *Archive) finishedBytes() int64 {
	a.mu.Lock()
	path := a.path
	a.mu.Unlock()

	if path == "" {
		return 0
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}

	return info.Size()
}

// build downloads every required mod into a new file. A mod that cannot be read
// fails the whole build: the archive exists so that a player can join, and one
// missing required mod would leave them where they started.
//
// The file is written as .part and renamed once complete, so a finished name
// always holds a whole archive; on any failure the .part is removed.
func (a *Archive) build(ctx context.Context, key string, mods []model.Mod) (path string, err error) {
	expected, known := expectedSize(mods)

	if a.maxBytes > 0 && known && expected > a.maxBytes {
		return "", errArchiveTooLarge
	}

	// What the build needs is the new archive plus the finished one a player may
	// still be downloading. Both are measured rather than assumed: the new file
	// cannot outgrow the cap, because cappedWriter stops the build there, and the
	// old one is on disk. Assuming twice the cap instead refused a build a small
	// volume had room for, whenever ModDB reported no size for one mod.
	need := expected
	if !known {
		need = a.maxBytes
	}
	if need <= 0 {
		// No size is known and no cap is set, so nothing bounds what this build
		// writes. Refusing beats filling the volume the rest of the service runs
		// on; MODS_ARCHIVE_MAX_BYTES is what removes the refusal.
		return "", fmt.Errorf("%w: no mod reported a size and the cap is 0", errArchiveUnbounded)
	}

	want := need + a.finishedBytes()
	if free, ok := freeBytes(a.dir); ok && free < want {
		return "", fmt.Errorf("%w: %d bytes free, %d wanted", errArchiveNoSpace, free, want)
	}

	path = filepath.Join(a.dir, archivePrefix+key+archiveSuffix)

	// The name being written carries a random part and is created exclusively.
	// A predictable name in a folder this process may not have created can be
	// replaced by a symlink first, and the build would then truncate whatever it
	// pointed at and publish the result on a public route. The random part still
	// matches isArchiveFile, so the sweep at start cleans up a crashed build.
	file, err := os.CreateTemp(a.dir, archivePrefix+key+"-*"+archiveSuffix+archivePartSuffix)
	if err != nil {
		return "", err
	}
	part := file.Name()

	defer func() {
		if err != nil {
			if removeErr := os.Remove(part); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				a.log.Warn("failed to remove a failed archive", zap.Error(removeErr))
			}
		}
	}()

	writer := zip.NewWriter(&cappedWriter{w: file, left: a.maxBytes})

	err = a.pack(ctx, writer, mods)
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}

	if err = os.Rename(part, path); err != nil {
		return "", err
	}

	return path, nil
}

// sweep removes every archive file in the folder except keep. Only names the
// archive itself uses are touched.
func (a *Archive) sweep(keep string) {
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		a.log.Warn("failed to read the archive folder", zap.Error(err))

		return
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isArchiveFile(name) {
			continue
		}

		path := filepath.Join(a.dir, name)
		if path == keep {
			continue
		}
		// A file still being written belongs to a build for another key. Only one
		// key is deduped at a time, so two builds run in parallel when the list
		// changes mid-download, and unlinking the other's part would make its
		// rename fail after every byte had been downloaded. The sweep at start
		// passes no keep and does remove them: nothing is being written then.
		if keep != "" && strings.HasSuffix(name, archivePartSuffix) {
			continue
		}

		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			a.log.Warn("failed to remove an old archive", zap.String("file", name), zap.Error(err))
		}
	}
}

// pack writes one entry per required mod, in display order. The entries are
// stored rather than deflated: a mod file is already a zip, so compressing it
// again buys a percent for the whole cost of running it through the packer.
func (a *Archive) pack(ctx context.Context, writer *zip.Writer, mods []model.Mod) error {
	taken := make(map[string]struct{}, len(mods))

	for _, mod := range mods {
		if mod.Release == nil {
			return fmt.Errorf("mod %s has no chosen version", mod.ModId)
		}

		header := &zip.FileHeader{
			Name:     entryName(mod.Release.FileName, mod.ModId+"-"+mod.Release.Version+".zip", taken),
			Method:   zip.Store,
			Modified: mod.Release.ReleasedAt,
		}
		header.SetMode(0o644)

		entry, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}

		body, err := a.moddb.Download(ctx, mod.Release.FileURL)
		if err != nil {
			return fmt.Errorf("download %s: %w", mod.ModId, err)
		}

		_, copyErr := io.Copy(entry, body)
		closeErr := body.Close()

		if copyErr != nil {
			return fmt.Errorf("write %s: %w", mod.ModId, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", mod.ModId, closeErr)
		}
	}

	return nil
}

// entryName is the name one mod's file carries inside the archive.
//
// The name comes from ModDB, so it is treated as hostile. A path component
// would let an extractor write outside the folder the player chose; a backslash
// or a leading separator means the same to a different extractor; an empty name
// produces an archive Windows refuses to open. Two mods that share a file name
// get the second one numbered, because an extractor keeps only one of two
// entries with the same name and the player would silently miss a required mod.
func entryName(fileName, fallback string, taken map[string]struct{}) string {
	name := path.Base(strings.ReplaceAll(strings.TrimSpace(fileName), "\\", "/"))
	switch name {
	case "", ".", "/", "..":
		name = fallback
	}

	// Keep the extension: the game's loader and the player's extractor both go
	// by it, so a name cut at the limit still ends in .zip. The cut backs up to
	// a rune boundary, because the zip writer flags the name as UTF-8 and half a
	// rune makes a strict extractor refuse the entry.
	if len(name) > maxEntryNameBytes {
		ext := path.Ext(name)
		keep := maxEntryNameBytes - len(ext)
		if keep <= 0 {
			ext, keep = "", maxEntryNameBytes
		}
		for keep > 0 && !utf8.RuneStart(name[keep]) {
			keep--
		}

		name = name[:keep] + ext
	}

	unique := name
	for n := 2; ; n++ {
		if _, used := taken[unique]; !used {
			break
		}

		ext := path.Ext(unique)
		unique = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), n, ext)
	}
	taken[unique] = struct{}{}

	return unique
}

// archiveKey names one set of required mods: their ids and the versions the
// server runs, in display order.
//
// A note the staff edited does not change what a player downloads, so it is not
// part of the key; a version does, which is what makes a re-versioned mod force
// a rebuild.
func archiveKey(mods []model.Mod) string {
	hash := sha256.New()

	for _, mod := range mods {
		_, _ = io.WriteString(hash, mod.ModId)
		_, _ = io.WriteString(hash, "\x00")
		_, _ = io.WriteString(hash, mod.Version())
		_, _ = io.WriteString(hash, "\x00")
		// The file the version points at, not only the version. ModDB can replace
		// the bytes behind one version, and a refresh that records the new file
		// has to force a rebuild: without this the archive keeps serving the
		// superseded build while the list already reports the new size.
		if mod.Release != nil {
			_, _ = io.WriteString(hash, mod.Release.FileURL)
			_, _ = io.WriteString(hash, "\x00")
			_, _ = io.WriteString(hash, mod.Release.FileName)
			_, _ = io.WriteString(hash, "\x00")
		}
	}

	return hex.EncodeToString(hash.Sum(nil))[:16]
}

// isArchiveFile reports whether name is one of the archive's own files.
func isArchiveFile(name string) bool {
	if !strings.HasPrefix(name, archivePrefix) {
		return false
	}

	return strings.HasSuffix(name, archiveSuffix) || strings.HasSuffix(name, archiveSuffix+archivePartSuffix)
}

// expectedSize sums the required mods' file sizes; known is false when any of
// them is unknown, and then the sum is not a size of anything.
func expectedSize(mods []model.Mod) (size int64, known bool) {
	known = true

	for _, mod := range mods {
		if mod.Release == nil || mod.Release.FileSize <= 0 {
			known = false

			continue
		}

		size += mod.Release.FileSize
	}

	return size, known
}

// cappedWriter fails once more than left bytes are written; left 0 means no
// cap. It catches an archive that outgrows the cap when ModDB did not say how
// big a file is, before it fills the disk.
type cappedWriter struct {
	w    io.Writer
	left int64
	used int64
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.left > 0 && c.used+int64(len(p)) > c.left {
		return 0, errArchiveTooLarge
	}

	n, err := c.w.Write(p)
	c.used += int64(n)

	return n, err
}
