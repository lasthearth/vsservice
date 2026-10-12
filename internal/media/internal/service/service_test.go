package service

import (
	"testing"

	mediav1 "github.com/lasthearth/vsservice/gen/media/v1"
)

func TestExtFromContentType(t *testing.T) {
	cases := map[string]string{
		"image/png":  ".png",
		"image/jpeg": ".jpg",
		"image/webp": ".webp",
		"":           ".webp",
		"text/plain": ".webp",
	}
	for ct, want := range cases {
		if got := extFromContentType(ct); got != want {
			t.Errorf("extFromContentType(%q) = %q, want %q", ct, got, want)
		}
	}
}

func TestPurposesConfigured(t *testing.T) {
	for purpose, cfg := range purposes {
		if cfg.bucket == "" {
			t.Errorf("purpose %v: empty bucket", purpose)
		}
		if cfg.maxSize <= 0 {
			t.Errorf("purpose %v: non-positive maxSize", purpose)
		}
		if len(cfg.contentTypes) == 0 {
			t.Errorf("purpose %v: no content types", purpose)
		}
	}
}

// TestPurposeScopes pins the per-purpose scope matrix. SETTLEMENT and WIKI
// carry no scope, so any authenticated user may stage an image for them; the
// wiki editor reaches WIKI through the player suggest flow (SubmitSuggestion is
// ScopeAuthenticated). DONATE_SHOP and NEWS stay admin-only. Publishing a page
// is gated by the wiki scopers (wiki:write / wiki:review), not by the upload.
func TestPurposeScopes(t *testing.T) {
	cases := map[mediav1.UploadPurpose]string{
		mediav1.UploadPurpose_UPLOAD_PURPOSE_DONATE_SHOP: "donate:shop:create",
		mediav1.UploadPurpose_UPLOAD_PURPOSE_SETTLEMENT:  "",
		mediav1.UploadPurpose_UPLOAD_PURPOSE_NEWS:        "news:create",
		mediav1.UploadPurpose_UPLOAD_PURPOSE_WIKI:        "",
	}
	for purpose, wantScope := range cases {
		cfg, ok := purposes[purpose]
		if !ok {
			t.Errorf("purpose %v: not configured", purpose)
			continue
		}
		if cfg.scope != wantScope {
			t.Errorf("purpose %v: scope = %q, want %q", purpose, cfg.scope, wantScope)
		}
	}
}
