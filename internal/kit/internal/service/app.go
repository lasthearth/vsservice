package service

import (
	kitv1 "github.com/lasthearth/vsservice/gen/kit/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/messaging"
	"github.com/lasthearth/vsservice/internal/pkg/messaging/mjetstream"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var _ kitv1.KitServiceServer = (*Service)(nil)

type Service struct {
	kitRepo        KitRepository
	assignmentRepo AssignmentRepository
	cfg            config.Config
	log            logger.Logger
	mapper         Mapper
	bus            *Bus
}

type Bus struct {
	kitGrantedPub  messaging.Publisher[KitGrantedEvent]
	kitClaimedSub  messaging.Subscriber[KitClaimedEvent]
	assignmentRepo AssignmentRepository
	log            logger.Logger
}

type Opts struct {
	fx.In
	Log            logger.Logger
	Config         config.Config
	AssignmentRepo AssignmentRepository
	Bus            *Bus
}

func NewFx(opts Opts) *Service {
	return New(
		nil,
		opts.AssignmentRepo,
		opts.Config,
		opts.Log,
		opts.Bus,
	)
}

func New(
	kitRepo KitRepository,
	assignmentRepo AssignmentRepository,
	cfg config.Config,
	log logger.Logger,
	bus *Bus,
) *Service {
	return &Service{
		kitRepo:        kitRepo,
		assignmentRepo: assignmentRepo,
		cfg:            cfg,
		log:            log,
		mapper:         nil,
		bus:            bus,
	}
}

func NewEventManager(
	nc *nats.Conn,
	l logger.Logger,
	assignmentRepo AssignmentRepository,
) *Bus {
	js, err := jetstream.New(nc)
	if err != nil {
		l.Error("failed to create jetstream", zap.Error(err))
		panic(err)
	}

	kgrt, err := mjetstream.NewPublisher[KitGrantedEvent](
		js,
		StreamName,
		KitGrantedEventSubject,
		l,
	)
	if err != nil {
		l.Error("failed to create publisher", zap.Error(err))
		panic(err)
	}

	consumerName := "kit-claimed-consumer"
	groupName := "kit-claimed-group"
	kclm, err := mjetstream.NewSubscriber[KitClaimedEvent](
		js,
		StreamName,
		KitClaimedEventSubject,
		consumerName,
		groupName,
		l,
	)
	if err != nil {
		l.Error("failed to create subscriber", zap.Error(err))
		panic(err)
	}

	return &Bus{
		kitGrantedPub:  kgrt,
		kitClaimedSub:  kclm,
		assignmentRepo: assignmentRepo,
		log:            l,
	}
}
