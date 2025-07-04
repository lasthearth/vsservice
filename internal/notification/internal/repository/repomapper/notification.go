// Code generated by github.com/jmattheis/goverter, DO NOT EDIT.
//go:build !goverter

package repomapper

import (
	dto "github.com/lasthearth/vsservice/internal/notification/internal/dto"
	model "github.com/lasthearth/vsservice/internal/notification/model"
	goverter "github.com/lasthearth/vsservice/internal/pkg/goverter"
)

type NotificationMapperImpl struct{}

func (c *NotificationMapperImpl) FromModel(source model.Notification) dto.Notification {
	var dtoNotification dto.Notification
	dtoNotification.UserId = source.UserId
	dtoNotification.Title = source.Title
	dtoNotification.Message = source.Message
	dtoNotification.State = string(source.State)
	return dtoNotification
}
func (c *NotificationMapperImpl) FromModels(source []model.Notification) []dto.Notification {
	var dtoNotificationList []dto.Notification
	if source != nil {
		dtoNotificationList = make([]dto.Notification, len(source))
		for i := 0; i < len(source); i++ {
			dtoNotificationList[i] = c.FromModel(source[i])
		}
	}
	return dtoNotificationList
}
func (c *NotificationMapperImpl) ToModel(source dto.Notification) model.Notification {
	var modelNotification model.Notification
	modelNotification.Id = goverter.ObjectIdToString(source.Model.Id)
	modelNotification.UserId = source.UserId
	modelNotification.Title = source.Title
	modelNotification.Message = source.Message
	modelNotification.State = model.NotificationState(source.State)
	modelNotification.CreatedAt = goverter.TimeToTime(source.Model.CreatedAt)
	modelNotification.UpdatedAt = goverter.TimeToTime(source.Model.UpdatedAt)
	return modelNotification
}
func (c *NotificationMapperImpl) ToModels(source []dto.Notification) []model.Notification {
	var modelNotificationList []model.Notification
	if source != nil {
		modelNotificationList = make([]model.Notification, len(source))
		for i := 0; i < len(source); i++ {
			modelNotificationList[i] = c.ToModel(source[i])
		}
	}
	return modelNotificationList
}
