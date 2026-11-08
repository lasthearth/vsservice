// Package logtoadmin tells who the server administrators are: the users that
// hold the Logto role named "admin". It is the one place that asks Logto, so
// every domain that notifies "the admins" agrees on who they are.
package logtoadmin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// RoleName is the Logto role whose holders are the administrators.
const RoleName = "admin"

// cacheTTL is how long a fetched list is reused. Admins change rarely, and
// every ticket message would otherwise cost two Logto calls.
const cacheTTL = 5 * time.Minute

// retryBackoff is how long a failed refresh is trusted before the next attempt.
// Without it a Logto outage makes every caller repeat a two-request fetch that
// holds the mutex for its whole duration.
const retryBackoff = 30 * time.Second

// pageSize is the Logto page size asked for, and maxPages bounds how many of
// them one refresh walks.
const (
	pageSize = 100
	maxPages = 20
)

// ErrRoleNotFound reports that Logto has no role named RoleName.
var ErrRoleNotFound = errors.New("logto role " + RoleName + " not found")

type Opts struct {
	fx.In
	Manager *tokenmanager.Manager
	Cfg     config.Config
	Logger  logger.Logger
}

// Directory lists the administrators, cached for cacheTTL.
type Directory struct {
	client  *http.Client
	baseURL string
	log     logger.Logger
	now     func() time.Time

	mu        sync.Mutex
	ids       []string
	fetchedAt time.Time
	failedAt  time.Time
}

func New(opts Opts) *Directory {
	return &Directory{
		client:  opts.Manager.Client(context.Background()),
		baseURL: opts.Cfg.SsoUrl,
		log:     opts.Logger.WithComponent("logto-admins"),
		now:     time.Now,
	}
}

// UserIDs returns the Logto user ids of the administrators. A failed refresh
// falls back to the last list it has, if any.
func (d *Directory) UserIDs(ctx context.Context) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	if d.ids != nil {
		if now.Sub(d.fetchedAt) < cacheTTL {
			return append([]string(nil), d.ids...), nil
		}
		// Stale, but a refresh just failed. Serving the old list beats making
		// every caller wait for Logto again.
		if now.Sub(d.failedAt) < retryBackoff {
			return append([]string(nil), d.ids...), nil
		}
	}

	ids, err := d.fetch(ctx)
	if err != nil {
		d.failedAt = now
		if d.ids != nil {
			d.log.Warn("failed to refresh admins, using the cached list", zap.Error(err))
			return append([]string(nil), d.ids...), nil
		}
		return nil, err
	}

	d.ids = ids
	d.fetchedAt = d.now()
	d.failedAt = time.Time{}
	return append([]string(nil), ids...), nil
}

func (d *Directory) fetch(ctx context.Context) ([]string, error) {
	var roles []struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	if err := getAll(ctx, d, "/api/roles", &roles); err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}

	roleID := ""
	for _, r := range roles {
		if r.Name == RoleName {
			roleID = r.Id
			break
		}
	}
	if roleID == "" {
		return nil, ErrRoleNotFound
	}

	var users []struct {
		Id string `json:"id"`
	}
	if err := getAll(ctx, d, "/api/roles/"+url.PathEscape(roleID)+"/users", &users); err != nil {
		return nil, fmt.Errorf("list role users: %w", err)
	}

	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Id)
	}
	return ids, nil
}

// getAll walks every page of a Logto list endpoint into dst. A short page ends
// the walk; hitting maxPages is logged, because the rest is then invisible.
// A plain function: Go methods take no type parameters.
func getAll[T any](ctx context.Context, d *Directory, path string, dst *[]T) error {
	for page := 1; page <= maxPages; page++ {
		var batch []T
		u := fmt.Sprintf("%s?page_size=%d&page=%d", path, pageSize, page)
		if err := d.get(ctx, u, &batch); err != nil {
			return err
		}
		*dst = append(*dst, batch...)
		if len(batch) < pageSize {
			return nil
		}
		if page == maxPages {
			d.log.Warn("logto list truncated",
				zap.String("path", path), zap.Int("pages", maxPages))
		}
	}
	return nil
}

func (d *Directory) get(ctx context.Context, pathAndQuery string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	u := d.baseURL + pathAndQuery
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("logto returned %d: %s", resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}
