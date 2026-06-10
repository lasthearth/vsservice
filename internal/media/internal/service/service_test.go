package service

import "testing"

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
