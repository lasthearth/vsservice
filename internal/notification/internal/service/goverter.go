package service

import (
	notificationv1 "github.com/lasthearth/vsservice/gen/notification/v1"
	"github.com/lasthearth/vsservice/internal/notification/model"
)

func StateToProto(state model.NotificationState) notificationv1.Notification_State {
	switch state {
	case model.NotificationStateUnread:
		return notificationv1.Notification_UNREAD
	case model.NotificationStateRead:
		return notificationv1.Notification_READ
	default:
		return notificationv1.Notification_STATE_UNSPECIFIED
	}
}
