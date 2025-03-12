package service

import (
	"context"
	"encoding/json"
	"github.com/ripls56/vsservice/cache"
	"github.com/ripls56/vsservice/event"
	v1 "github.com/ripls56/vsservice/gen/protos/v1"
	"github.com/ripls56/vsservice/logger"
	"github.com/samber/lo"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ v1.VintageServiceServer = (*VsApiV1)(nil)
var _ CacheManager = (*cache.Ristretto)(nil)

type CacheManager interface {
	Get(key string) (any, error)
	Set(key string, value any) error
	Delete(key string) error
}

type VsApiV1 struct {
	log   logger.Logger
	bus   *event.Bus
	cache CacheManager
}

type VsApiV1Opts struct {
	fx.In
	Log   logger.Logger
	Bus   *event.Bus
	Cache CacheManager
}

func NewVsApiV1(opts VsApiV1Opts) *VsApiV1 {
	return &VsApiV1{
		log:   opts.Log,
		bus:   opts.Bus,
		cache: opts.Cache,
	}
}

func (v *VsApiV1) GetOnlinePlayersCount(ctx context.Context, req *emptypb.Empty) (*v1.PlayersCountResponse, error) {
	var playerCount event.PlayerCountEvent

	eventType := event.PlayerCount

	cached, err := v.cache.Get(string(eventType))

	if err != nil {
		return nil, nil
	}

	body, ok := cached.(json.RawMessage)
	if !ok {
		return nil, status.Error(
			codes.Internal,
			"failed to cast cached value",
		)
	}

	err = json.Unmarshal(body, &playerCount)
	if err != nil {
		return nil, err
	}

	return &v1.PlayersCountResponse{
		Count: int32(playerCount.Count),
	}, nil
}

func (v *VsApiV1) GetGameTime(ctx context.Context, e *emptypb.Empty) (*v1.TimeResponse, error) {
	var worldTime event.WorldTimeEvent

	eventType := event.WorldTime

	cached, err := v.cache.Get(string(eventType))

	if err != nil {
		return nil, nil
	}

	body, ok := cached.(json.RawMessage)
	if !ok {
		return nil, status.Error(
			codes.Internal,
			"failed to cast cached value",
		)
	}

	err = json.Unmarshal(body, &worldTime)
	if err != nil {
		return nil, err
	}

	return &v1.TimeResponse{
		FormattedTime: worldTime.FormattedTime,
	}, nil
}

func (v *VsApiV1) StreamGameTime(empty *emptypb.Empty, stream grpc.ServerStreamingServer[v1.TimeResponse]) error {
	var worldTime event.WorldTimeEvent

	eventType := event.WorldTime

	sub, id := v.bus.Subscribe(eventType)
	defer v.bus.Unsubscribe(eventType, id)

	// need to process data inside of subscription loop
	fn := func(ev event.Event) error {
		err := json.Unmarshal(ev.Data, &worldTime)
		if err != nil {
			return err
		}

		err = stream.Send(&v1.TimeResponse{
			FormattedTime: worldTime.FormattedTime,
		})
		if err != nil {
			return err
		}

		return nil
	}

	cached, err := v.cache.Get(string(eventType))

	if err != nil {
		err = stream.Send(nil)
		err = v.streamLoop(
			stream.Context(),
			sub,
			fn,
		)
		if err != nil {
			return err
		}
	}

	body, ok := cached.(json.RawMessage)
	if !ok {
		return status.Error(
			codes.Internal,
			"failed to cast cached value",
		)
	}

	err = json.Unmarshal(body, &worldTime)
	if err != nil {
		return err
	}

	err = stream.Send(&v1.TimeResponse{
		FormattedTime: worldTime.FormattedTime,
	})
	if err != nil {
		return err
	}

	err = v.streamLoop(
		stream.Context(),
		sub,
		fn,
	)

	if err != nil {
		return err
	}

	return nil
}

func (v *VsApiV1) StreamOnlinePlayersCount(e *emptypb.Empty, stream grpc.ServerStreamingServer[v1.PlayersCountResponse]) error {
	var playerCount event.PlayerCountEvent

	eventType := event.PlayerCount

	sub, id := v.bus.Subscribe(eventType)
	defer v.bus.Unsubscribe(eventType, id)

	// need to process data inside of subscription loop
	fn := func(ev event.Event) error {
		err := json.Unmarshal(ev.Data, &playerCount)
		if err != nil {
			return err
		}

		err = stream.Send(&v1.PlayersCountResponse{
			Count: int32(playerCount.Count),
		})
		if err != nil {
			return err
		}

		return nil
	}

	cached, err := v.cache.Get(string(eventType))

	if err != nil {
		err = stream.Send(nil)
		err = v.streamLoop(
			stream.Context(),
			sub,
			fn,
		)
		if err != nil {
			return err
		}
	}

	body, ok := cached.(json.RawMessage)
	if !ok {
		return status.Error(
			codes.Internal,
			"failed to cast cached value",
		)
	}

	err = json.Unmarshal(body, &playerCount)
	if err != nil {
		return err
	}

	err = stream.Send(&v1.PlayersCountResponse{
		Count: int32(playerCount.Count),
	})
	if err != nil {
		return err
	}

	err = v.streamLoop(
		stream.Context(),
		sub,
		fn,
	)

	if err != nil {
		return err
	}

	return nil
}

func (v *VsApiV1) GetOnlinePlayersList(ctx context.Context, e *emptypb.Empty) (*v1.PlayersListResponse, error) {
	var playerList event.PlayerListEvent

	eventType := event.PlayerList

	cached, err := v.cache.Get(string(eventType))

	if err != nil {
		return nil, nil
	}

	body, ok := cached.(json.RawMessage)
	if !ok {
		return nil, status.Error(
			codes.Internal,
			"failed to cast cached value",
		)
	}

	err = json.Unmarshal(body, &playerList)
	if err != nil {
		return nil, err
	}

	return &v1.PlayersListResponse{
		PlayerNames: lo.Map(
			playerList.Players,
			func(item event.Player, index int) string {
				return item.Name
			},
		),
	}, nil
}

func (v *VsApiV1) StreamOnlinePlayersList(e *emptypb.Empty, stream grpc.ServerStreamingServer[v1.PlayersListResponse]) error {
	var playerList event.PlayerListEvent

	eventType := event.PlayerList

	sub, id := v.bus.Subscribe(eventType)
	defer v.bus.Unsubscribe(eventType, id)

	// need to process data inside of subscription loop
	fn := func(ev event.Event) error {
		err := json.Unmarshal(ev.Data, &playerList)
		if err != nil {
			return err
		}

		err = stream.Send(&v1.PlayersListResponse{
			PlayerNames: lo.Map(
				playerList.Players,
				func(item event.Player, index int) string {
					return item.Name
				},
			),
		})
		if err != nil {
			return err
		}

		return nil
	}

	cached, err := v.cache.Get(string(eventType))

	if err != nil {
		err = stream.Send(nil)
		err = v.streamLoop(
			stream.Context(),
			sub,
			fn,
		)
		if err != nil {
			return err
		}
	}

	body, ok := cached.(json.RawMessage)
	if !ok {
		return status.Error(
			codes.Internal,
			"failed to cast cached value",
		)
	}

	err = json.Unmarshal(body, &playerList)
	if err != nil {
		return err
	}

	err = stream.Send(&v1.PlayersListResponse{
		PlayerNames: lo.Map(
			playerList.Players,
			func(item event.Player, index int) string {
				return item.Name
			},
		),
	})
	if err != nil {
		return err
	}

	err = v.streamLoop(
		stream.Context(),
		sub,
		fn,
	)

	if err != nil {
		return err
	}

	return nil
}

// streamLoop used to processing and caching events from chan
func (v *VsApiV1) streamLoop(ctx context.Context, sub <-chan event.Event, fn func(ev event.Event) error) error {
	for {
		select {
		case ev := <-sub:
			err := fn(ev)
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
