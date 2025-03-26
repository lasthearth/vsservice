package fetcher

import (
	"context"
	"encoding/json"
	"fmt"
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
	doneCh chan struct{}
}

func New(log logger.Logger, cfg config.Config) *Fetcher {
	return &Fetcher{
		log:    log,
		cfg:    cfg,
		doneCh: make(chan struct{}),
	}
}

// Fetch player stats by provided names from in game api
func (f *Fetcher) Fetch(ctx context.Context, ch chan<- *httpdto.Stats) {
	select {
	case <-ctx.Done():
		f.log.Info("fetcher stopped")
		return
	case <-f.doneCh:
		f.log.Info("fetcher stopped")
		return
	default:
		names, err := f.getAllPlayersNames()
		if err != nil {
			f.log.Error("failed to get players names", zap.Error(err))
		}
		for _, name := range names {
			stats, err := f.getPlayerStats(name)
			if err != nil {
				f.log.Error("failed to get player stats", zap.Error(err))
				continue
			}
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

	resp, err := http.Get(url)
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
	var stats httpdto.Stats

	reqUrl := "stats"
	url := fmt.Sprintf("%s/%s/%s", f.cfg.VsAPIUrl, reqUrl, name)

	resp, err := http.Get(url)
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

	err = json.Unmarshal(buf, &stats)
	if err != nil {
		return nil, ErrUnmarshalJSON
	}

	return &stats, nil
}

func (f *Fetcher) checkStatusCode(resp *http.Response) error {
	if resp.StatusCode >= 400 {
		return ErrHTTPStatusNotOK
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
