package service

import (
	"testing"
	"time"

	"github.com/lasthearth/vsservice/internal/mod/internal/model"
)

// fixedNow is the clock the fixtures are stamped with.
var fixedNow = time.Date(2026, time.October, 14, 15, 0, 0, 0, time.UTC)

// requiredMod is one required mod at a chosen version.
func requiredMod(modId, version string, size int64) model.Mod {
	release := model.Release{
		Version:  version,
		FileURL:  "https://moddbcdn.vintagestory.at/" + modId + "-" + version + ".zip",
		FileName: modId + "-" + version + ".zip",
		FileSize: size,
	}

	return model.Build(modId, model.Snapshot{
		Name:     modId,
		Releases: []model.Release{release},
	}, model.AudienceRequired, "", &release, fixedNow)
}

func TestArchiveSizeIsUnknownWhenOneModIs(t *testing.T) {
	known := requiredMod("known", "1.0.0", 1024)
	unknown := model.Build("unknown", model.Snapshot{}, model.AudienceRequired, "", nil, fixedNow)

	got := archiveProto([]model.Mod{known, unknown})
	if got == nil {
		t.Fatal("archive is unset")
	}
	if got.GetCount() != 2 {
		t.Errorf("count = %d, want 2", got.GetCount())
	}
	if got.GetSize() != 0 {
		t.Errorf("size = %d, want 0 when one of them is unknown", got.GetSize())
	}
}

func TestArchiveSizeIsUnknownWhenAFileSizeIsZero(t *testing.T) {
	got := archiveProto([]model.Mod{requiredMod("a", "1.0.0", 1024), requiredMod("b", "1.0.0", 0)})

	if got.GetSize() != 0 {
		t.Errorf("size = %d, want 0 when one file size was never measured", got.GetSize())
	}
}

func TestArchiveDescribesTheRequiredModsOnly(t *testing.T) {
	optional := requiredMod("optional", "1.0.0", 4096)
	optional.SetAudience(model.AudienceOptional)

	got := archiveProto([]model.Mod{requiredMod("required", "1.0.0", 1024), optional})

	if got.GetCount() != 1 || got.GetSize() != 1024 {
		t.Errorf("archive = %d mods / %d bytes, want the required one only", got.GetCount(), got.GetSize())
	}
	if !got.GetUpdatedAt().AsTime().Equal(fixedNow) {
		t.Errorf("updated_at = %v, want %v", got.GetUpdatedAt().AsTime(), fixedNow)
	}
}

func TestArchiveKeyFollowsTheVersionAndNotTheNote(t *testing.T) {
	first := requiredMod("carryon", "1.0.0", 1024)
	noted := requiredMod("carryon", "1.0.0", 1024)
	noted.SetNote("нужен всем")

	if archiveKey([]model.Mod{first}) != archiveKey([]model.Mod{noted}) {
		t.Error("a note the staff edited changed the archive key")
	}

	bumped := requiredMod("carryon", "2.0.0", 1024)
	if archiveKey([]model.Mod{first}) == archiveKey([]model.Mod{bumped}) {
		t.Error("a new version did not change the archive key")
	}

	other := requiredMod("stonequarry", "1.0.0", 1024)
	if archiveKey([]model.Mod{first}) == archiveKey([]model.Mod{other}) {
		t.Error("another mod did not change the archive key")
	}

	if archiveKey([]model.Mod{first, other}) == archiveKey([]model.Mod{other, first}) {
		t.Error("the display order is part of the key")
	}
}

func TestArchiveKeyIsStable(t *testing.T) {
	first := []model.Mod{requiredMod("carryon", "1.0.0", 1024), requiredMod("stonequarry", "2.3.4", 10)}
	second := []model.Mod{requiredMod("carryon", "1.0.0", 1024), requiredMod("stonequarry", "2.3.4", 10)}

	if archiveKey(first) != archiveKey(second) {
		t.Error("the same list produced two keys")
	}
}

func TestSameSetTakesOnlyAPermutation(t *testing.T) {
	stored := []model.Mod{requiredMod("a", "1.0.0", 1), requiredMod("b", "1.0.0", 1)}

	cases := map[string]struct {
		ids  []string
		want bool
	}{
		"the same order": {[]string{"a", "b"}, true},
		"reversed":       {[]string{"b", "a"}, true},
		"a missing id":   {[]string{"a"}, false},
		"an extra id":    {[]string{"a", "b", "c"}, false},
		"a repeat":       {[]string{"a", "a"}, false},
		"an unknown id":  {[]string{"a", "c"}, false},
		"nothing":        {nil, false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := sameSet(stored, tc.ids); got != tc.want {
				t.Errorf("sameSet(%v) = %v, want %v", tc.ids, got, tc.want)
			}
		})
	}
}
