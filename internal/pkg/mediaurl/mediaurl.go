package mediaurl

import (
	"errors"
	"net/url"
	"strings"

	"github.com/lasthearth/vsservice/internal/pkg/config"
)

var ErrInvalidURL = errors.New("media url is not allowed")

type Validator struct {
	allowedHosts map[string]struct{}
}

func New(cfg config.Config) *Validator {
	hosts := make(map[string]struct{})
	if u, err := url.Parse(cfg.CdnUrl); err == nil && u.Host != "" {
		hosts[u.Host] = struct{}{}
	}
	for _, h := range cfg.MediaAllowedHosts {
		if h = strings.TrimSpace(h); h != "" {
			hosts[h] = struct{}{}
		}
	}
	return &Validator{allowedHosts: hosts}
}

func (v *Validator) Validate(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return ErrInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrInvalidURL
	}
	if u.Host == "" {
		return ErrInvalidURL
	}
	if _, ok := v.allowedHosts[u.Host]; !ok {
		return ErrInvalidURL
	}
	return nil
}
