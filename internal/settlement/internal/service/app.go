package service

import (
	"net/http"

	"github.com/eapache/go-resiliency/retrier"
	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.uber.org/fx"
)

var _ settlementv1.SettlementServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Client   *http.Client
	Log      logger.Logger
	Retrier  *retrier.Retrier
	DbRepo   SettlementRepository
	Mapper   Mapper
	MediaURL *mediaurl.Validator
	Notifier *notificationuc.Create
	Mail     mailcompose.MailComposer
}

type Service struct {
	client   *http.Client
	log      logger.Logger
	dbRepo   SettlementRepository
	retrier  *retrier.Retrier
	mapper   Mapper
	mediaUrl *mediaurl.Validator
	notifier *notificationuc.Create
	// notices sends a site notification to one user; nil when the notification
	// stack is not wired.
	notices UserNotifier
	// mail composes in-game mails (the notifier block, upgrade notices).
	mail mailcompose.MailComposer
}

func New(opts Opts) *Service {
	var notices UserNotifier
	if opts.Notifier != nil {
		notices = createNotifier{uc: opts.Notifier}
	}
	return &Service{
		notices:  notices,
		mail:     opts.Mail,
		client:   opts.Client,
		log:      opts.Log,
		dbRepo:   opts.DbRepo,
		retrier:  opts.Retrier,
		mapper:   opts.Mapper,
		mediaUrl: opts.MediaURL,
		notifier: opts.Notifier,
	}
}
