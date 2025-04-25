package sso

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-faster/errors"
)

func (r *Repository) UpdateUserProfileNick(ctx context.Context, userID, nickname string) error {
	url := fmt.Sprintf("%s/api/users/%s", r.config.SsoUrl, userID)

	v := struct {
		Profile struct {
			Nickname string `json:"nickname"`
		} `json:"profile"`
	}{
		Profile: struct {
			Nickname string `json:"nickname"`
		}{
			Nickname: nickname,
		},
	}

	encoded, err := json.Marshal(v)
	if err != nil {
		return ErrMarshalJSON
	}

	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(encoded))
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
