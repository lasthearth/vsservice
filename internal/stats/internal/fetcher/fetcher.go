package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-faster/errors"
	"github.com/ripls56/vsservice/internal/pkg/config"
	"github.com/ripls56/vsservice/internal/pkg/logger"
	"github.com/ripls56/vsservice/internal/stats/internal/dto/httpdto"
	"go.uber.org/zap"
	"io"
	"net/http"
	"time"
)

// Fetcher fetch player stats
type Fetcher struct {
	log    logger.Logger
	cfg    config.Config
	client *http.Client
	doneCh chan struct{}
}

func New(log logger.Logger, cfg config.Config, client *http.Client) *Fetcher {
	return &Fetcher{
		log:    log.WithComponent("fetcher"),
		cfg:    cfg,
		client: client,
		doneCh: make(chan struct{}),
	}
}

// Fetch player stats by provided names from in game api
func (f *Fetcher) Fetch(ctx context.Context, ch chan<- *httpdto.Stats) {
	l := f.log.WithMethod("fetch")
	select {
	case <-ctx.Done():
		l.Info("fetcher stopped")
		return
	case <-f.doneCh:
		l.Info("fetcher stopped")
		return
	default:
		names, err := f.getAllPlayersNames()
		if err != nil {
			l.Error("failed to get players names", zap.Error(err))
		}
		for _, name := range names {
			stats, err := f.getPlayerStats(name)
			if err != nil {
				l.With(zap.String("name", name)).Error("failed to get player stats", zap.Error(err))
				continue
			}

			l.With(zap.String("name", name)).Info("successfully get player stats")
			ch <- stats
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (f *Fetcher) Stop() {
	f.log.Info("stop fetcher")
	f.doneCh <- struct{}{}
}

func (f *Fetcher) getAllPlayersNames() ([]string, error) {
	names := make([]string, 0)

	reqUrl := "players"
	url := fmt.Sprintf("%s/%s", f.cfg.VsAPIUrl, reqUrl)

	resp, err := f.client.Get(url)
	if err != nil {
		return nil, ErrHTTPRequestFailed
	}

	err = f.checkStatusCode(resp)
	if err != nil {
		return nil, err
	}

	buf, err := f.readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(buf, &names)
	if err != nil {
		return nil, ErrUnmarshalJSON
	}

	return names, nil
}

func (f *Fetcher) getPlayerStats(name string) (*httpdto.Stats, error) {
	l := f.log.WithMethod("getPlayerStats")
	var stats httpdto.Stats

	reqUrl := "stats"
	url := fmt.Sprintf("%s/%s/%s", f.cfg.VsAPIUrl, reqUrl, name)

	resp, err := f.client.Get(url)
	if err != nil {
		l.Error("failed http request", zap.Error(err))
		return nil, ErrHTTPRequestFailed
	}

	err = f.checkStatusCode(resp)
	if err != nil {
		l.Error("http status code not ok", zap.Error(err))
		return nil, err
	}

	buf, err := f.readBody(resp.Body)
	if err != nil {
		l.Error("read body failed", zap.Error(err))
		return nil, err
	}

	err = json.Unmarshal(buf, &stats)
	if err != nil {
		l.Error("serialize failed", zap.Error(err))
		return nil, ErrUnmarshalJSON
	}

	return &stats, nil
}

func (f *Fetcher) checkStatusCode(resp *http.Response) error {
	if resp.StatusCode >= 400 {
		return errors.Wrap(ErrHTTPStatusNotOK, resp.Status)
	}
	return nil
}

func (f *Fetcher) readBody(rd io.ReadCloser) ([]byte, error) {
	method := "readBody"
	buf, err := io.ReadAll(rd)
	defer func(rd io.ReadCloser) {
		err := rd.Close()
		if err != nil {
			f.log.WithMethod(method).Error("close failed", zap.Error(err))
		}
	}(rd)

	if err != nil {
		return nil, ErrReadResponseBody
	}
	return buf, nil
}
