package sso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-faster/errors"
	httpdto "github.com/lasthearth/vsservice/internal/rules/internal/dto/http"
)

func (r *Repository) GetUserRoles(ctx context.Context, userId string) ([]httpdto.Role, error) {
	existingRolesUrl := fmt.Sprintf("%s/api/users/%s/roles", r.cfg.SsoUrl, userId)

	req, err := http.NewRequest(http.MethodGet, existingRolesUrl, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrReadResponseBody
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}
	var roles []httpdto.Role
	err = json.Unmarshal(body, &roles)
	if err != nil {
		return nil, ErrUnmarshalJSON
	}

	return roles, nil
}

func (r *Repository) GetRoles(ctx context.Context) ([]httpdto.Role, error) {
	getRolesUrl := fmt.Sprintf("%s/api/roles", r.cfg.SsoUrl)

	req, err := http.NewRequest(http.MethodGet, getRolesUrl, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrReadResponseBody
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}
	var roles []httpdto.Role
	err = json.Unmarshal(body, &roles)
	if err != nil {
		return nil, ErrUnmarshalJSON
	}

	return roles, nil
}

func (r *Repository) UpdateUserRoles(ctx context.Context, userId string, roleIds []string) error {
	updateRolesUrl := fmt.Sprintf("%s/api/users/%s/roles", r.cfg.SsoUrl, userId)

	rBody := struct {
		RoleIds []string `json:"roleIds"`
	}{
		RoleIds: roleIds,
	}

	encoded, err := json.Marshal(rBody)
	if err != nil {
		return ErrMarshalJSON
	}

	req, err := http.NewRequest(http.MethodPut, updateRolesUrl, bytes.NewBuffer(encoded))
	if err != nil {
		return ErrFailedCreateReq
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return ErrHTTPRequestFailed
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrReadResponseBody
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errors.Wrap(ErrHTTPStatusNotOK, string(body))
	}

	return nil
}
