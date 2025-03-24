package service

import (
	"context"
	"encoding/json"
	"fmt"
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/model/player"
	"github.com/ripls56/vsservice/model/playerstats"
	"github.com/ripls56/vsservice/model/worldtime"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"net/http"
	"strings"
)

var _ v1.VintageServiceServer = (*VsApiV1)(nil)

const (
	//vsserverUrl = "http://vsserver:5000"
	vsserverUrl = "http://localhost:5000"
)

type CacheManager interface {
	Get(key string) (any, error)
	Set(key string, value any) error
}

func (v *VsApiV1) GetOnlinePlayersCount(ctx context.Context, req *emptypb.Empty) (*v1.PlayersCountResponse, error) {
	var playerCount player.Count

	reqUrl := "players/count"
	url := fmt.Sprintf("%s/%s", vsserverUrl, reqUrl)

	resp, err := http.Get(url)
	if err != nil {
		return nil, status.Error(codes.Internal, ErrHTTPRequestFailed.Error())
	}

	err = v.checkStatusCode(resp)
	if err != nil {
		return nil, err
	}

	buf, err := v.readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(buf, &playerCount)
	if err != nil {
		return nil, status.Error(codes.Internal, ErrUnmarshalJSON.Error())
	}

	return &v1.PlayersCountResponse{
		Count: int32(playerCount.Count),
	}, nil
}

func (v *VsApiV1) GetGameTime(ctx context.Context, e *emptypb.Empty) (*v1.TimeResponse, error) {
	var time worldtime.Time

	reqUrl := "time"
	url := fmt.Sprintf("%s/%s", vsserverUrl, reqUrl)

	resp, err := http.Get(url)
	if err != nil {
		return nil, status.Error(codes.Internal, ErrHTTPRequestFailed.Error())
	}

	err = v.checkStatusCode(resp)
	if err != nil {
		return nil, err
	}

	buf, err := v.readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(buf, &time)
	if err != nil {
		return nil, status.Error(codes.Internal, ErrUnmarshalJSON.Error())
	}

	return &v1.TimeResponse{
		FormattedTime: time.FormattedTime,
	}, nil
}

func (v *VsApiV1) GetPlayerStats(ctx context.Context, req *v1.PlayerStatsRequest) (*v1.PlayerStatsResponse, error) {
	var stats playerstats.Stats

	reqUrl := "stats"
	url := fmt.Sprintf("%s/%s/%s", vsserverUrl, reqUrl, req.Name)

	resp, err := http.Get(url)
	if err != nil {
		return nil, status.Error(codes.Internal, ErrHTTPRequestFailed.Error())
	}

	err = v.checkStatusCode(resp)
	if err != nil {
		return nil, err
	}

	buf, err := v.readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(buf, &stats)
	if err != nil {
		return nil, status.Error(codes.Internal, ErrUnmarshalJSON.Error())
	}

	deaths := lo.Map(stats.Deaths, func(item playerstats.Death, index int) *v1.PlayerStatsResponse_Death {
		return &v1.PlayerStatsResponse_Death{
			Cause:      strings.ToLower(item.Cause),
			EntityName: item.EntityName,
		}
	})

	return &v1.PlayerStatsResponse{
		Id:            stats.ID,
		Name:          stats.Name,
		DeathCount:    int32(stats.DeathCount),
		Deaths:        deaths,
		HoursPlayed:   stats.HoursPlayed,
		LastOnline:    stats.LastOnline,
		PlayersKilled: int32(stats.PlayersKilled),
	}, nil
}

func (v *VsApiV1) checkStatusCode(resp *http.Response) error {
	if resp.StatusCode >= 400 {
		return status.Error(codes.Internal, ErrHTTPStatusNotOK.Error())
	}
	return nil
}

func (v *VsApiV1) readBody(r io.ReadCloser) ([]byte, error) {
	buf, err := io.ReadAll(r)
	defer r.Close()
	if err != nil {
		return nil, status.Error(codes.Internal, ErrReadResponseBody.Error())
	}
	return buf, nil
}
