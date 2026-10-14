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
	"sync"
	"time"

	"github.com/lasthearth/vsservice/internal/mod/internal/model"
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
)

// Archive packs the required mods into one zip and serves it over plain HTTP.
//
// The archive is built once per set of (id, version) pairs and kept in a file
// under the service's temporary directory. Rebuilding it per request would pull
// every mod out of ModDB again for every player, and the same list is what all
// of them download.
type Archive struct {
	mods  RequiredMods
	moddb *moddb.Client
	log   logger.Logger

	// dir holds the built archives. It is created with the process and left to
	// the operating system to reclaim: an archive is a cache, and nothing that
	// reads one depends on it surviving a restart.
	dir string

	mu    sync.Mutex
	key   string
	path  string
	group singleflight.Group
}

// NewArchive prepares the directory the packed archives live in.
func NewArchive(mods RequiredMods, client *moddb.Client, log logger.Logger) (*Archive, error) {
	dir, err := os.MkdirTemp("", "vsservice-mods-")
	if err != nil {
		return nil, fmt.Errorf("create the archive directory: %w", err)
	}

	return &Archive{
		mods:  mods,
		moddb: client,
		log:   log.WithComponent("archive"),
		dir:   dir,
	}, nil
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
		a.log.Error("failed to build the archive", zap.Error(err))
		http.Error(w, "could not build the archive", http.StatusBadGateway)

		return
	}

	//nolint:gosec // G304: the path is a file this service built in its own
	// temporary directory, never one a caller names.
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
	previous := a.path
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

		// The previous archive is for versions nobody is served any more.
		if previous != "" && previous != path {
			if err := os.Remove(previous); err != nil && !errors.Is(err, os.ErrNotExist) {
				a.log.Warn("failed to remove the previous archive", zap.Error(err))
			}
		}

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
func (a *Archive) build(ctx context.Context, key string, mods []model.Mod) (string, error) {
	file, err := os.CreateTemp(a.dir, key+"-*.zip")
	if err != nil {
		return "", err
	}

	path := file.Name()
	writer := zip.NewWriter(file)

	err = a.pack(ctx, writer, mods)
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		if removeErr := os.Remove(path); removeErr != nil {
			a.log.Warn("failed to remove a failed archive", zap.Error(removeErr))
		}

		return "", err
	}

	return path, nil
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
