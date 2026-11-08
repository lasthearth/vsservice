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

// pageSize is the Logto page size asked for; the server has far fewer roles
// and admins than that.
const pageSize = 100

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

	if d.ids != nil && d.now().Sub(d.fetchedAt) < cacheTTL {
		return append([]string(nil), d.ids...), nil
	}

	ids, err := d.fetch(ctx)
	if err != nil {
		if d.ids != nil {
			d.log.Warn("failed to refresh admins, using the cached list", zap.Error(err))
			return append([]string(nil), d.ids...), nil
		}
		return nil, err
	}

	d.ids = ids
	d.fetchedAt = d.now()
	return append([]string(nil), ids...), nil
}

func (d *Directory) fetch(ctx context.Context) ([]string, error) {
	var roles []struct {
		Id   string `json:"id"`
		Name string `json:"name"`
	}
	if err := d.get(ctx, "/api/roles", &roles); err != nil {
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
	if err := d.get(ctx, "/api/roles/"+url.PathEscape(roleID)+"/users", &users); err != nil {
		return nil, fmt.Errorf("list role users: %w", err)
	}

	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Id)
	}
	return ids, nil
}

func (d *Directory) get(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	u := fmt.Sprintf("%s%s?page_size=%d", d.baseURL, path, pageSize)
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
