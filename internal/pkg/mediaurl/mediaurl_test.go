package mediaurl_test

import (
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
)

func newValidator() *mediaurl.Validator {
	return mediaurl.New(config.Config{
		CdnUrl:            "https://cdn.test",
		MediaAllowedHosts: []string{"i.imgur.com"},
	})
}

func TestValidate(t *testing.T) {
	v := newValidator()
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"cdn host https", "https://cdn.test/donate-shop/a.webp", false},
		{"cdn host http", "http://cdn.test/donate-shop/a.webp", false},
		{"allowlisted host", "https://i.imgur.com/abc.png", false},
		{"disallowed host", "https://evil.example/x.png", true},
		{"bad scheme", "ftp://cdn.test/x", true},
		{"garbage", "not a url", true},
		{"empty", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := v.Validate(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.url, err)
			}
		})
	}
}
