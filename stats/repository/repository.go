package repository

import (
	"encoding/json"
	"fmt"
	"github.com/ripls56/vsservice/stats/model"
	"io"
	"net/http"
)

func (r *Repository) Get(name string) (*model.Stats, error) {
	var stats model.Stats

	reqUrl := "stats"
	url := fmt.Sprintf("%s/%s/%s", r.cfg.VsAPIUrl, reqUrl, name)

	resp, err := http.Get(url)
	if err != nil {
		return nil, ErrHTTPRequestFailed
	}

	err = r.checkStatusCode(resp)
	if err != nil {
		return nil, err
	}

	buf, err := r.readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(buf, &stats)
	if err != nil {
		return nil, ErrUnmarshalJSON
	}

	return &stats, nil
}

func (r *Repository) checkStatusCode(resp *http.Response) error {
	if resp.StatusCode >= 400 {
		return ErrHTTPStatusNotOK
	}
	return nil
}

func (r *Repository) readBody(rd io.ReadCloser) ([]byte, error) {
	buf, err := io.ReadAll(rd)
	defer rd.Close()
	if err != nil {
		return nil, ErrReadResponseBody
	}
	return buf, nil
}
