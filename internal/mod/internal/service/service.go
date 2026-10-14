package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/mod/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/moddb"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// archiveURL is where the archive is served. The proto repeats it, because the
// site links the archive the list answer points at.
const archiveURL = "/v1/mods/archive.zip"

// ListMods implements modv1.ModServiceServer.
//
// The public list is what a player installs: the required mods and the optional
// ones, in the order the staff arranged, with the archive of the required ones.
func (s *Service) ListMods(ctx context.Context, _ *modv1.ListModsRequest) (*modv1.ListModsResponse, error) {
	mods, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	visible := make([]model.Mod, 0, len(mods))
	for _, mod := range mods {
		if mod.Audience == model.AudienceRequired || mod.Audience == model.AudienceOptional {
			visible = append(visible, mod)
		}
	}

	return &modv1.ListModsResponse{
		Mods:    s.protoMods(visible),
		Archive: archiveProto(visible),
	}, nil
}

// ListModsAdmin implements modv1.ModServiceServer.
//
// Staff see the whole list, HIDDEN mods included, each with the releases ModDB
// last reported: that is what the admin tab chooses a version from.
func (s *Service) ListModsAdmin(
	ctx context.Context,
	_ *modv1.ListModsAdminRequest,
) (*modv1.ListModsAdminResponse, error) {
	mods, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	return &modv1.ListModsAdminResponse{Mods: s.mapper.ToModProtos(mods)}, nil
}

// LookupMod implements modv1.ModServiceServer.
//
// Nothing is stored: the admin is looking at what ModDB has before deciding
// who needs it and which version the server should run.
func (s *Service) LookupMod(
	ctx context.Context,
	req *modv1.LookupModRequest,
) (*modv1.LookupModResponse, error) {
	found, err := s.fetch(ctx, req.GetQuery())
	if err != nil {
		return nil, err
	}

	// No version is chosen yet, so none is measured and the newest one is
	// offered.
	looked := model.Build(
		modID(found.Releases),
		s.snapshotOf(ctx, found, ""),
		model.AudienceUnspecified,
		"",
		nil,
		s.now().UTC(),
	)

	return &modv1.LookupModResponse{
		Mod:      s.protoMod(looked),
		Releases: ToReleaseProtos(looked.Releases),
	}, nil
}

// AddMod implements modv1.ModServiceServer.
func (s *Service) AddMod(ctx context.Context, req *modv1.AddModRequest) (*modv1.Mod, error) {
	audience := audienceOf(req.GetAudience())
	if audience == model.AudienceUnspecified {
		return nil, ierror.ErrAudience
	}

	note, err := normalizeNote(req.GetNote())
	if err != nil {
		return nil, err
	}

	found, err := s.fetch(ctx, req.GetQuery())
	if err != nil {
		return nil, err
	}

	snapshot := s.snapshotOf(ctx, found, req.GetVersion())

	chosen, ok := model.FindRelease(snapshot.Releases, req.GetVersion())
	if !ok {
		return nil, ierror.ErrUnknownVersion
	}

	added := model.Build(modID(found.Releases), snapshot, audience, note, &chosen, s.now().UTC())

	if err := s.repo.Create(ctx, &added); err != nil {
		return nil, err
	}

	// Read back what was stored: the answer carries the position the list gave
	// the mod and the write time the document has.
	saved, err := s.repo.Get(ctx, added.ModId)
	if err != nil {
		return nil, err
	}

	return s.protoMod(*saved), nil
}

// UpdateMod implements modv1.ModServiceServer.
//
// Unset fields stay as they are; a version that changes has to exist on ModDB,
// which is what makes this call reach ModDB at all.
func (s *Service) UpdateMod(ctx context.Context, req *modv1.UpdateModRequest) (*modv1.Mod, error) {
	modId := strings.TrimSpace(req.GetId())
	if modId == "" {
		return nil, ierror.ErrNotInList
	}

	// The new version is read before the write: a version ModDB does not list
	// is refused without touching the stored mod, and the file size is measured
	// once here rather than inside the retried update. The snapshot travels with
	// it, because the version may be one the stored snapshot never held.
	var (
		fresh    *model.Release
		snapshot *model.Snapshot
	)
	if req.Version != nil {
		stored, err := s.repo.Get(ctx, modId)
		if err != nil {
			return nil, err
		}

		found, err := s.fetch(ctx, stored.ModId)
		if err != nil {
			return nil, err
		}

		read := s.snapshotOf(ctx, found, req.GetVersion())

		release, ok := model.FindRelease(read.Releases, req.GetVersion())
		if !ok {
			return nil, ierror.ErrUnknownVersion
		}

		fresh, snapshot = &release, &read
	}

	var wanted *model.Audience
	if req.Audience != nil {
		value := audienceOf(req.GetAudience())
		if value == model.AudienceUnspecified {
			return nil, ierror.ErrAudience
		}

		wanted = &value
	}

	var note *string
	if req.Note != nil {
		value, err := normalizeNote(req.GetNote())
		if err != nil {
			return nil, err
		}

		note = &value
	}

	updated, err := s.repo.Save(ctx, modId, func(_ context.Context, mod *model.Mod) (*model.Mod, error) {
		// The snapshot first: it re-points the chosen release at the stored
		// version, and SetRelease then names the new one and its size.
		if snapshot != nil {
			mod.SetSnapshot(*snapshot)
		}
		if fresh != nil {
			mod.SetRelease(*fresh)
		}
		if wanted != nil {
			mod.SetAudience(*wanted)
		}
		if note != nil {
			mod.SetNote(*note)
		}

		return mod, nil
	})
	if err != nil {
		return nil, err
	}

	return s.protoMod(*updated), nil
}

// DeleteMod implements modv1.ModServiceServer.
func (s *Service) DeleteMod(ctx context.Context, req *modv1.DeleteModRequest) (*emptypb.Empty, error) {
	if err := s.repo.Delete(ctx, strings.TrimSpace(req.GetId())); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// ReorderMods implements modv1.ModServiceServer.
//
// The order is a permutation of the stored mods or it is refused: an order that
// lost a mod would leave it at whatever position it had, silently out of the
// sequence the admin arranged.
func (s *Service) ReorderMods(
	ctx context.Context,
	req *modv1.ReorderModsRequest,
) (*modv1.ListModsAdminResponse, error) {
	stored, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	if !sameSet(stored, req.GetIds()) {
		return nil, ierror.ErrOrder
	}

	if err := s.repo.Reorder(ctx, req.GetIds()); err != nil {
		return nil, err
	}

	return s.ListModsAdmin(ctx, &modv1.ListModsAdminRequest{})
}

// RefreshMods implements modv1.ModServiceServer.
//
// Every mod is read from ModDB again, but the versions the server runs are
// never changed: a refresh updates what ModDB says, not what the staff decided.
// A mod ModDB cannot answer for keeps its old data and the call still succeeds.
func (s *Service) RefreshMods(
	ctx context.Context,
	_ *modv1.RefreshModsRequest,
) (*modv1.ListModsAdminResponse, error) {
	mods, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	for _, stored := range mods {
		found, err := s.fetch(ctx, stored.ModId)
		if err != nil {
			s.log.Warn("failed to read a mod from ModDB, keeping what is stored",
				zap.String("id", stored.ModId), zap.Error(err))
			continue
		}

		snapshot := s.snapshotOf(ctx, found, stored.Version())

		_, err = s.repo.Save(ctx, stored.ModId, func(_ context.Context, mod *model.Mod) (*model.Mod, error) {
			mod.SetSnapshot(snapshot)
			return mod, nil
		})
		if err != nil {
			s.log.Warn("failed to store the refreshed mod",
				zap.String("id", stored.ModId), zap.Error(err))
		}
	}

	return s.ListModsAdmin(ctx, &modv1.ListModsAdminRequest{})
}

// fetch reads a mod from ModDB, turning every outcome into what the API
// documents: a bad reference, a mod ModDB does not know, and ModDB not
// answering are three different answers for the admin.
func (s *Service) fetch(ctx context.Context, query string) (*moddb.Mod, error) {
	ref, err := moddb.ParseRef(query)
	if err != nil {
		return nil, ierror.ErrBadQuery
	}

	found, err := s.moddb.GetMod(ctx, ref)

	switch {
	case errors.Is(err, moddb.ErrNotFound):
		return nil, ierror.ErrModNotFound
	case errors.Is(err, moddb.ErrUnavailable):
		return nil, ierror.ErrUnavailable
	case err != nil:
		return nil, err
	}

	// A mod without releases names no version the server could run and has no
	// modidstr to be stored under.
	if len(found.Releases) == 0 {
		return nil, ierror.ErrNoReleases
	}

	return found, nil
}

// snapshotOf turns a ModDB answer into what the site stores. The chosen version
// is measured again, because its file may have been replaced since the mod was
// added and the archive's size has to describe what a player would download.
func (s *Service) snapshotOf(ctx context.Context, found *moddb.Mod, chosen string) model.Snapshot {
	releases := newestReleases(found.Releases)

	for i, release := range releases {
		if release.Version == chosen {
			releases[i].SetFileSize(s.moddb.FileSize(ctx, release.FileURL))
		}
	}

	return model.Snapshot{
		Name:     found.Name,
		Author:   found.Author,
		LogoURL:  found.LogoFile,
		ModDBURL: moddb.PageURL(found.URLAlias, found.AssetID),
		Side:     sideOf(found.Side),
		Summary:  moddb.Summary(found.Text),
		Releases: releases,
	}
}

// newestReleases maps ModDB's releases into the snapshot the site keeps. ModDB
// answers newest first and the snapshot keeps that order; at most MaxReleases
// of them are stored.
func newestReleases(releases []moddb.Release) []model.Release {
	if len(releases) > model.MaxReleases {
		releases = releases[:model.MaxReleases]
	}

	out := make([]model.Release, 0, len(releases))
	for _, release := range releases {
		out = append(out, model.Release{
			Version:      release.ModVersion,
			GameVersions: release.Tags,
			FileURL:      release.MainFile,
			FileName:     release.FileName,
			ReleasedAt:   release.ReleasedAt(),
		})
	}

	return out
}

// modID is the mod's own id. ModDB states it on every release rather than on
// the mod, so the newest release that carries one names the mod.
func modID(releases []moddb.Release) string {
	for _, release := range releases {
		if release.ModIDStr != "" {
			return release.ModIDStr
		}
	}

	return ""
}

// archiveProto describes the archive of the required mods in mods, or nil when
// there is nothing to pack. A size that is unknown anywhere makes the whole
// total unknown: half a total would be a lie the site renders.
func archiveProto(mods []model.Mod) *modv1.ModArchive {
	var (
		count  int32
		size   int64
		newest time.Time
		known  = true
	)

	for _, mod := range mods {
		if mod.Audience != model.AudienceRequired {
			continue
		}

		count++

		if mod.Release == nil || mod.Release.FileSize <= 0 {
			known = false
		} else {
			size += mod.Release.FileSize
		}

		if mod.UpdatedAt.After(newest) {
			newest = mod.UpdatedAt
		}
	}

	if count == 0 {
		return nil
	}

	if !known {
		size = 0
	}

	var updatedAt *timestamppb.Timestamp
	if !newest.IsZero() {
		updatedAt = timestamppb.New(newest)
	}

	return &modv1.ModArchive{
		Url:       archiveURL,
		Count:     count,
		Size:      size,
		UpdatedAt: updatedAt,
	}
}

// normalizeNote trims the staff's one-liner and checks its two limits. An empty
// result is a mod without a note.
func normalizeNote(note string) (string, error) {
	trimmed := strings.TrimSpace(note)

	if utf8.RuneCountInString(trimmed) > model.MaxNoteRunes {
		return "", ierror.ErrNote
	}

	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return "", ierror.ErrNote
		}
	}

	return trimmed, nil
}

// sameSet reports whether ids names every stored mod exactly once.
func sameSet(stored []model.Mod, ids []string) bool {
	if len(ids) != len(stored) {
		return false
	}

	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			return false
		}

		seen[id] = struct{}{}
	}

	for _, mod := range stored {
		if _, ok := seen[mod.ModId]; !ok {
			return false
		}
	}

	return true
}

// protoMod maps a stored mod for the API. The release history is served only
// where the proto documents it, so every other answer drops it.
func (s *Service) protoMod(mod model.Mod) *modv1.Mod {
	out := s.mapper.ToModProto(mod)
	out.Releases = nil

	return out
}

// protoMods is protoMod over a list.
func (s *Service) protoMods(mods []model.Mod) []*modv1.Mod {
	out := s.mapper.ToModProtos(mods)
	for _, mod := range out {
		mod.Releases = nil
	}

	return out
}
