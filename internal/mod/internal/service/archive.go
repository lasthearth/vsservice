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
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	a.mu.Lock()
	if a.key == key && a.path != "" {
		path := a.path
		a.mu.Unlock()

		return path, nil
	}
	a.mu.Unlock()

	built, err, _ := a.group.Do(key, func() (any, error) {
		// Another request may have finished this very key while this one waited
		// for its turn.
		a.mu.Lock()
		if a.key == key && a.path != "" {
			path := a.path
			a.mu.Unlock()

			return path, nil
		}
		a.mu.Unlock()

		buildCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), archiveBuildTimeout)
		defer cancel()

		path, err := a.build(buildCtx, key, mods)
		if err != nil {
			return "", err
		}

		a.mu.Lock()
		a.key, a.path = key, path
		a.mu.Unlock()

		// Whatever else is in the folder is for versions nobody is served any
		// more. A player still downloading an older archive keeps reading it:
		// removing an open file only unlinks its name.
		a.sweep(path)

		return path, nil
	})
	if err != nil {
		return "", err
	}

	return built.(string), nil
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

	// A zip of stored entries is the files plus a few hundred bytes of headers
	// each; twice the files leaves room for that and for the old archive a
	// player may still be downloading.
	need := expected
	if !known {
		need = a.maxBytes
	}
	if free, ok := freeBytes(a.dir); ok && need > 0 && free < 2*need {
		return "", fmt.Errorf("%w: %d bytes free, %d wanted", errArchiveNoSpace, free, 2*need)
	}

	path = filepath.Join(a.dir, archivePrefix+key+archiveSuffix)
	part := path + archivePartSuffix

	//nolint:gosec // G304: the name is built from the archive key, a hex hash.
	file, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}

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

		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			a.log.Warn("failed to remove an old archive", zap.String("file", name), zap.Error(err))
		}
	}
}

// pack writes one entry per required mod, in display order. The entries are
// stored rather than deflated: a mod file is already a zip, so compressing it
// again buys a percent for the whole cost of running it through the packer.
func (a *Archive) pack(ctx context.Context, writer *zip.Writer, mods []model.Mod) error {
	for _, mod := range mods {
		if mod.Release == nil {
			return fmt.Errorf("mod %s has no chosen version", mod.ModId)
		}

		header := &zip.FileHeader{
			Name:     mod.Release.FileName,
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
