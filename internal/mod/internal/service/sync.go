package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/moddb"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

const (
	// defaultsForSyncer are the values MODS_* take when the environment says
	// nothing. They are repeated here so a unit test that builds a Syncer from
	// a zero config still behaves like a deployment.
	defaultModsBucket    = "server-mods"
	defaultManifestKey   = "manifest.json"
	defaultSyncInterval  = time.Minute
	manifestVersion      = 1
	maxManifestBytes     = 8 << 20
	modsSyncTimeout      = 30 * time.Second
	sideUniversal        = "universal"
	sha256HexLength      = 64
	cacheGroupKey        = "server-mods-manifest"
	manifestLogComponent = "sync"
)

var (
	// errManifestVersion is a manifest this service does not understand. The
	// stored list is kept: a newer game server must not empty /mods.
	errManifestVersion = errors.New("unsupported manifest version")
	// errManifestInvalid is a manifest that parsed but carries nothing usable: a
	// duplicate id or a non-empty list where no entry passed validation. It is
	// treated like a version this service cannot read, never as "the server has
	// no required mods". An empty list is valid and means exactly that.
	errManifestInvalid = errors.New("manifest carries no usable mods")
)

// ObjectStorage is the subset of pkg/storage the sync reads: the manifest, and
// a release file when no CDN URL can serve it.
type ObjectStorage interface {
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
}

// BucketStore is the subset of pkg/storage the startup bootstrap needs.
type BucketStore interface {
	BucketExists(ctx context.Context, bucket string) (bool, error)
	CreateBucket(ctx context.Context, bucket string) error
	MakeBucketPublic(ctx context.Context, bucket string) error
}

// Syncer keeps the stored list in step with the manifest the game server
// writes to MinIO.
//
// The manifest is the game server's statement of which mods a client must
// install. The sync owns those documents: it adds the ones it does not know,
// rewrites the ones it does, and removes the ones the manifest dropped. Mods the
// staff added are never read here and never written here.
type Syncer struct {
	repo    Repository
	objects ObjectStorage
	log     logger.Logger

	bucket      string
	manifestKey string
	cdnURL      string
	interval    time.Duration

	// now is a field so a test can pin the clock the cache and the stored write
	// time are taken from.
	now func() time.Time
	// afterSync, when set by a test, runs once a pass has finished including the
	// cache timestamp. It gives an out-of-band Refresh test a happens-before
	// edge on every field Sync touched.
	afterSync func()

	// group makes every concurrent caller share one read of MinIO: the ticker,
	// the boot pass and a request that finds the cache stale must not each fetch
	// the same manifest, and must not apply two merges at once.
	group singleflight.Group

	// refreshing keeps the out-of-band refresh single. It is not part of the
	// group because the goroutine that owns it must not be created per request.
	refreshing atomic.Bool

	mu       sync.Mutex
	syncedAt time.Time
}

// SyncerOpts is the fx input of the sync; a test fills the fields directly.
type SyncerOpts struct {
	fx.In

	Repo    Repository
	Objects ObjectStorage
	Logger  logger.Logger
	Config  config.Config
}

// NewSyncer builds the manifest sync.
func NewSyncer(opts SyncerOpts) *Syncer {
	bucket := opts.Config.ModsBucket
	if bucket == "" {
		bucket = defaultModsBucket
	}

	key := opts.Config.ModsManifestKey
	if key == "" {
		key = defaultManifestKey
	}

	interval := opts.Config.ModsSyncInterval
	if interval <= 0 {
		interval = defaultSyncInterval
	}

	log := opts.Logger.WithComponent(manifestLogComponent)

	// One warning at start, not one per mod: a missing CDN is a configuration
	// mistake, and the archive falls back to reading the objects from MinIO.
	if opts.Config.CdnUrl == "" {
		log.Warn("CDN_URL is empty: server mods carry no direct file URL, " +
			"and their files are only served by the archive")
	}

	return &Syncer{
		repo:        opts.Repo,
		objects:     opts.Objects,
		log:         log,
		bucket:      bucket,
		manifestKey: key,
		cdnURL:      opts.Config.CdnUrl,
		interval:    interval,
		now:         time.Now,
	}
}

// Run performs the first sync and then one per interval until ctx is done. fx
// starts it from OnStart; every failure is logged and retried on the next tick,
// because /mods must keep serving the stored list while MinIO or the game
// server is down.
func (s *Syncer) Run(ctx context.Context) {
	s.syncShared(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncShared(ctx)
		}
	}
}

// Refresh syncs when the cached list is older than the interval.
//
// It runs out of band on purpose: ListMods calls it, and a manifest read from a
// blackholed MinIO has a 30 s budget. A public request must answer from Mongo at
// once instead of waiting for that budget, and one failed read must not be
// retried by every request that follows it.
func (s *Syncer) Refresh(ctx context.Context) {
	if !s.stale() {
		return
	}

	// One refresh at a time. The flag is cleared when the pass ends, so a
	// failed pass is retried by the next request or tick, not by all of them.
	if !s.refreshing.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer s.refreshing.Store(false)

		s.syncShared(context.WithoutCancel(ctx))
	}()
}

// syncShared runs one pass through the singleflight group, so the ticker, the
// boot pass and a lazy refresh never apply two merges at the same time.
func (s *Syncer) syncShared(ctx context.Context) {
	_, _, _ = s.group.Do(cacheGroupKey, func() (any, error) {
		return nil, s.Sync(ctx)
	})
}

// Sync reads the manifest once and applies it to the stored list.
//
// A manifest that cannot be read or validated changes nothing: the error is
// logged at Warn and returned, and ListMods keeps answering from Mongo. This is
// what stops the first deployment - with the bucket not created yet - from
// erasing the whole /mods page.
func (s *Syncer) Sync(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, modsSyncTimeout)
	defer cancel()

	err := s.sync(ctx)

	// The attempt counts even when it failed: a MinIO that is down must not be
	// retried by every request to /mods, and the ticker retries it on its own
	// schedule anyway.
	s.mu.Lock()
	s.syncedAt = s.now()
	s.mu.Unlock()

	if s.afterSync != nil {
		s.afterSync()
	}

	if err != nil {
		s.log.Warn("failed to sync the server mods, keeping the stored list", zap.Error(err))

		return err
	}

	return nil
}

// sync is one read-validate-merge pass.
func (s *Syncer) sync(ctx context.Context) error {
	doc, err := s.readManifest(ctx)
	if err != nil {
		return err
	}

	entries, err := s.entriesOf(doc)
	if err != nil {
		return err
	}

	stored, err := s.repo.List(ctx)
	if err != nil {
		return err
	}

	byID := make(map[string]model.Mod, len(stored))
	for _, mod := range stored {
		byID[mod.ModId] = mod
	}

	wanted := make(map[string]struct{}, len(entries))
	desired := make([]model.Mod, 0, len(entries))
	added := 0

	for _, entry := range entries {
		wanted[entry.modId] = struct{}{}

		if existing, ok := byID[entry.modId]; ok {
			// The copy is the stored mod: the manifest rewrites what the game
			// server says and leaves the staff's note and the arranged position
			// where they are.
			updated := existing
			updated.SetServerManifest(entry.snapshot, entry.release)
			desired = append(desired, updated)

			continue
		}

		// A new mod goes after every stored one; the repository assigns the
		// position, because it is the one that knows the list.
		desired = append(desired, model.NewServerManifest(
			entry.modId, entry.snapshot, entry.release, 0, s.now().UTC()))
		added++
	}

	removed := make([]string, 0)
	for _, mod := range stored {
		if !mod.IsServer() {
			// Admin-sourced and legacy OPTIONAL mods are the staff's; the
			// manifest never mentions them and the sync never touches them.
			continue
		}

		if _, ok := wanted[mod.ModId]; !ok {
			removed = append(removed, mod.ModId)
		}
	}

	if err := s.repo.Sync(ctx, desired, removed); err != nil {
		return err
	}

	s.log.Info("synced the server mods",
		zap.Int("mods", len(desired)),
		zap.Int("added", added),
		zap.Int("removed", len(removed)),
	)

	return nil
}

// stale reports whether the last successful sync is older than the interval.
func (s *Syncer) stale() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.now().Sub(s.syncedAt) >= s.interval
}

// manifest is the document VintageAPI writes to MinIO after uploading the mod
// files. Only the fields the site renders are read.
type manifest struct {
	Version     int           `json:"version"`
	GeneratedAt time.Time     `json:"generated_at"`
	GameVersion string        `json:"game_version"`
	Mods        []manifestMod `json:"mods"`
}

// manifestMod is one mod of the manifest.
type manifestMod struct {
	ModID            string    `json:"modid"`
	Name             string    `json:"name"`
	Authors          []string  `json:"authors"`
	Description      string    `json:"description"`
	Version          string    `json:"version"`
	Side             string    `json:"side"`
	RequiredOnClient bool      `json:"required_on_client"`
	Filename         string    `json:"filename"`
	Size             int64     `json:"size"`
	SHA256           string    `json:"sha256"`
	FileKey          string    `json:"file_key"`
	IconKey          string    `json:"icon_key"`
	UploadedAt       time.Time `json:"uploaded_at"`
}

// serverEntry is one validated manifest mod, ready to be applied to the list.
type serverEntry struct {
	modId    string
	snapshot model.Snapshot
	release  model.Release
}

// readManifest fetches and decodes the manifest.
func (s *Syncer) readManifest(ctx context.Context) (*manifest, error) {
	body, err := s.objects.GetObject(ctx, s.bucket, s.manifestKey)
	if err != nil {
		return nil, fmt.Errorf("read the mods manifest: %w", err)
	}

	defer func() {
		if closeErr := body.Close(); closeErr != nil {
			s.log.Warn("failed to close the mods manifest", zap.Error(closeErr))
		}
	}()

	var doc manifest

	// The manifest is a small document; the limit keeps a broken or hostile
	// object from filling memory before the decoder gives up.
	if err := json.NewDecoder(io.LimitReader(body, maxManifestBytes)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode the mods manifest: %w", err)
	}

	return &doc, nil
}

// skipReason is why one manifest entry was left out. It carries no mod id or
// name: a rejected entry may be a server-only mod, and its identity must not
// reach a log line. Reasons are aggregated and logged once per pass.
type skipReason string

const (
	reasonNoID         skipReason = "no modid"
	reasonNotForClient skipReason = "a client does not need it"
	reasonNoVersion    skipReason = "no version"
	reasonNoFileName   skipReason = "no file name"
	reasonBadFileKey   skipReason = "unusable file key"
	reasonBadIconKey   skipReason = "unusable icon key"
	reasonBadSHA256    skipReason = "malformed sha256"
)

// entriesOf validates the manifest and turns it into the mods the sync stores.
//
// Everything that makes the document untrustworthy fails the whole pass: an
// unknown version, a duplicate id, a non-empty list with no usable entry. A
// single bad entry is skipped and counted, because one broken mod must not
// freeze the rest of the list.
//
// An empty list is a valid manifest: the game server writes one when no client
// mod is required, and the site must then show no required mods. A missing
// manifest stays an error, so "never synced" cannot empty the list.
func (s *Syncer) entriesOf(doc *manifest) ([]serverEntry, error) {
	if doc.Version != manifestVersion {
		return nil, fmt.Errorf("%w: %d", errManifestVersion, doc.Version)
	}

	if doc.Mods == nil {
		// An absent value is not "zero client mods": a document that lost its
		// list must not empty the site. An explicit [] is the server's statement
		// that no client mod is required and is handled below.
		return nil, fmt.Errorf("%w: the mod list is missing", errManifestInvalid)
	}

	if len(doc.Mods) == 0 {
		return []serverEntry{}, nil
	}

	seen := make(map[string]struct{}, len(doc.Mods))
	entries := make([]serverEntry, 0, len(doc.Mods))
	skipped := make(map[skipReason]int, len(doc.Mods))

	for _, mod := range doc.Mods {
		modId := strings.TrimSpace(mod.ModID)
		if modId == "" {
			skipped[reasonNoID]++

			continue
		}

		if _, duplicate := seen[modId]; duplicate {
			// The message names no id: the duplicate key may be a server-only
			// mod, and the log is where that name would leak.
			return nil, fmt.Errorf("%w: duplicate modid", errManifestInvalid)
		}
		seen[modId] = struct{}{}

		entry, reason, ok := s.entryOf(doc, modId, mod)
		if !ok {
			skipped[reason]++

			continue
		}

		entries = append(entries, entry)
	}

	if len(skipped) > 0 {
		// Reasons only: a rejected entry may be a server-only mod whose id and
		// name the site must never log.
		s.log.Warn("skipped manifest entries",
			zap.Any("reasons", skipCounts(skipped)),
			zap.Int("kept", len(entries)),
		)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: no entry passed validation", errManifestInvalid)
	}

	return entries, nil
}

// skipCounts turns the per-reason counters into a loggable shape without the
// reasons' map type leaking into the log call.
func skipCounts(skipped map[skipReason]int) map[string]int {
	counts := make(map[string]int, len(skipped))
	for reason, count := range skipped {
		counts[string(reason)] = count
	}

	return counts
}

// entryOf validates one manifest mod and maps it into the domain model shape.
// ok is false for an entry the sync must skip, and reason says why without
// naming the mod.
func (s *Syncer) entryOf(doc *manifest, modId string, mod manifestMod) (serverEntry, skipReason, bool) {
	// The manifest is written by another service, so the client-only filter is
	// checked again here. A server-side mod in this list would put NDA code on
	// a public CDN path, which is the one mistake that cannot be undone.
	if !mod.RequiredOnClient || strings.TrimSpace(mod.Side) != sideUniversal {
		return serverEntry{}, reasonNotForClient, false
	}

	version := strings.TrimSpace(mod.Version)
	if version == "" {
		return serverEntry{}, reasonNoVersion, false
	}

	fileName := strings.TrimSpace(mod.Filename)
	if fileName == "" {
		return serverEntry{}, reasonNoFileName, false
	}

	if !validObjectKey(mod.FileKey) {
		return serverEntry{}, reasonBadFileKey, false
	}

	if mod.IconKey != "" && !validObjectKey(mod.IconKey) {
		return serverEntry{}, reasonBadIconKey, false
	}

	if !validSHA256(mod.SHA256) {
		return serverEntry{}, reasonBadSHA256, false
	}

	releasedAt := mod.UploadedAt
	if releasedAt.IsZero() {
		releasedAt = doc.GeneratedAt
	}

	gameVersions := make([]string, 0, 1)
	if gameVersion := strings.TrimSpace(doc.GameVersion); gameVersion != "" {
		gameVersions = append(gameVersions, gameVersion)
	}

	release := model.Release{
		Version:      version,
		GameVersions: gameVersions,
		FileURL:      objectURL(s.cdnURL, s.bucket, mod.FileKey),
		FileName:     fileName,
		FileSize:     mod.Size,
		SHA256:       strings.ToLower(strings.TrimSpace(mod.SHA256)),
		FileKey:      mod.FileKey,
		ReleasedAt:   releasedAt,
	}

	name := strings.TrimSpace(mod.Name)
	if name == "" {
		name = modId
	}

	return serverEntry{
		modId: modId,
		snapshot: model.Snapshot{
			Name:     name,
			Author:   strings.Join(mod.Authors, ", "),
			LogoURL:  objectURL(s.cdnURL, s.bucket, mod.IconKey),
			Side:     model.SideBoth,
			Summary:  moddb.Summary(mod.Description),
			Releases: []model.Release{release},
		},
		release: release,
	}, "", true
}

// objectURL builds the public CDN URL of one object.
//
// Every segment is escaped: Vintage Story mod files routinely carry spaces and
// other characters a browser cannot put in a URL as they are. An empty CDN URL
// yields no URL at all rather than a broken relative one; the archive then reads
// the object from MinIO itself.
func objectURL(cdnURL, bucket, key string) string {
	key = strings.TrimSpace(key)
	if cdnURL == "" || key == "" {
		return ""
	}

	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}

	return strings.TrimSuffix(cdnURL, "/") + "/" + bucket + "/" + strings.Join(segments, "/")
}

// validObjectKey reports whether key names an object inside the bucket and
// cannot be read as a path on any machine. It is checked because the key becomes
// a URL the site renders and, with no CDN, an object lookup.
func validObjectKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return false
	}

	for segment := range strings.SplitSeq(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}

	return true
}

// validSHA256 reports whether digest is empty or 64 hexadecimal characters. The
// manifest may omit the digest; a malformed one is data the sync must not store.
func validSHA256(digest string) bool {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return true
	}

	if len(digest) != sha256HexLength {
		return false
	}

	for _, r := range digest {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}

	return true
}
