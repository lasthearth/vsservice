// Package moddb reads mods from the Vintage Story mod database.
//
// The site cannot talk to ModDB from the browser: ModDB sends no CORS headers,
// so the backend is the only one that can look a mod up.
package moddb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ModDB states its own status in the body, as a string, and it is not always
// the HTTP status: a mod it does not know is answered 200 with "404" inside.
const (
	bodyStatusOK       = "200"
	bodyStatusNotFound = "404"
)

// BaseURL is the ModDB site; the API lives under /api.
const BaseURL = "https://mods.vintagestory.at"

// userAgent identifies this service to ModDB.
const userAgent = "lasthearth-vsservice"

// apiTimeout bounds one ModDB API call. ModDB answers in milliseconds when it
// answers at all; ten seconds is where the site should say "try later" instead
// of holding the admin's request open.
const apiTimeout = 10 * time.Second

// maxAPIBytes caps one API answer. A mod's description can be long, but not
// megabytes long: the limit keeps a misbehaving upstream from filling memory.
const maxAPIBytes = 8 << 20

var (
	// ErrNotFound reports that ModDB knows no mod under the reference.
	ErrNotFound = errors.New("moddb: no such mod")
	// ErrUnavailable reports that ModDB did not answer in time, or answered with
	// something the API does not define.
	ErrUnavailable = errors.New("moddb: moddb did not answer")
	// ErrInvalidRef reports a query that names no ModDB mod: empty text, a link
	// to another site, or a path ModDB does not serve.
	ErrInvalidRef = errors.New("moddb: not a moddb reference")
)

// Client reads ModDB. The zero value is not usable; use New.
type Client struct {
	baseURL string
	api     *http.Client
	files   *http.Client
}

// New returns a client that times out an API call after ten seconds.
func New() *Client {
	return &Client{
		baseURL: BaseURL,
		api:     &http.Client{Timeout: apiTimeout},
		// A mod file is megabytes and the gateway streams it to the player, so
		// a download is bounded by the request context, not by a client
		// timeout: cutting off a slow download would break the archive.
		files: &http.Client{},
	}
}

// Mod is one mod as ModDB describes it.
type Mod struct {
	ModID    int       `json:"modid"`
	AssetID  int       `json:"assetid"`
	Name     string    `json:"name"`
	Author   string    `json:"author"`
	Text     string    `json:"text"`
	LogoFile string    `json:"logofile"`
	URLAlias string    `json:"urlalias"`
	Side     string    `json:"side"`
	Releases []Release `json:"releases"`
}

// Release is one released version of a mod.
type Release struct {
	ModIDStr   string   `json:"modidstr"`
	ModVersion string   `json:"modversion"`
	Tags       []string `json:"tags"`
	MainFile   string   `json:"mainfile"`
	FileName   string   `json:"filename"`
	Created    string   `json:"created"`
}

// ReleasedAt parses ModDB's "2026-10-14 13:43:04" stamp, which is UTC. A stamp
// it cannot parse is a zero time, which the site shows as unknown rather than
// as 1970.
func (r Release) ReleasedAt() time.Time {
	t, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(r.Created))
	if err != nil {
		return time.Time{}
	}

	return t.UTC()
}

// GetMod reads one mod. ref is an asset id, a url alias or a mod id — the three
// keys mods.vintagestory.at/api/mod/{key} accepts.
func (c *Client) GetMod(ctx context.Context, ref string) (*Mod, error) {
	endpoint := c.baseURL + "/api/mod/" + url.PathEscape(ref)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ErrInvalidRef
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.api.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	// ModDB answers 200 with {"statuscode":"404","mod":null} for a mod it does
	// not know, so the status in the body decides, not the HTTP status.
	var payload struct {
		StatusCode string `json:"statuscode"`
		Mod        *Mod   `json:"mod"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIBytes)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	if payload.StatusCode == bodyStatusNotFound || payload.Mod == nil {
		return nil, ErrNotFound
	}
	if payload.StatusCode != bodyStatusOK {
		return nil, fmt.Errorf("%w: status %s", ErrUnavailable, payload.StatusCode)
	}

	return payload.Mod, nil
}

// Download opens one mod file for reading. The caller closes it.
func (c *Client) Download(ctx context.Context, fileURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, ErrInvalidRef
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.files.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	return resp.Body, nil
}

// FileSize asks ModDB how large the file behind fileURL is.
//
// ModDB's API does not carry the size, so the only source is the Content-Length
// of a HEAD. Anything that keeps the answer from arriving — a failure, a
// missing header, a zero — is 0, which the API documents as "unknown" and which
// callers must not treat as an error.
func (c *Client) FileSize(ctx context.Context, fileURL string) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fileURL, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.api.Do(req)
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK || resp.ContentLength <= 0 {
		return 0
	}

	return resp.ContentLength
}
