package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ripls56/vsservice/event"
	v1 "github.com/ripls56/vsservice/gen/protos/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"net/http"
)

var _ v1.VintageServiceServer = (*VsApiV1)(nil)

const (
	vsserverUrl = "http://vsserver:5000/"
)

type CacheManager interface {
	Get(key string) (any, error)
	Set(key string, value any) error
}

func (v *VsApiV1) GetOnlinePlayersCount(ctx context.Context, req *emptypb.Empty) (*v1.PlayersCountResponse, error) {
	var playerCount event.PlayerCountEvent

	reqUrl := "players/count"
	resp, err := http.Get(fmt.Sprintf("%s%s", vsserverUrl, reqUrl))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil, status.Error(codes.FailedPrecondition, "no content")
	}

	buf, err := io.ReadAll(resp.Body)

	err = json.Unmarshal(buf, &playerCount)
	if err != nil {
		return nil, err
	}

	return &v1.PlayersCountResponse{
		Count: int32(playerCount.Count),
	}, nil
}

func (v *VsApiV1) GetGameTime(ctx context.Context, e *emptypb.Empty) (*v1.TimeResponse, error) {
	var worldTime event.WorldTimeEvent

	reqUrl := "time"
	resp, err := http.Get(fmt.Sprintf("%s%s", vsserverUrl, reqUrl))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil, status.Error(codes.FailedPrecondition, "no content")
	}

	buf, err := io.ReadAll(resp.Body)

	err = json.Unmarshal(buf, &worldTime)
	if err != nil {
		return nil, err
	}

	return &v1.TimeResponse{
		FormattedTime: worldTime.FormattedTime,
	}, nil
}
