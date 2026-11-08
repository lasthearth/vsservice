package model

import "time"

const BroadcastUserId = "broadcast"

type Notification struct {
	Id      string
	UserId  string `validate:"required"`
	Title   string `validate:"required"`
	Message string `validate:"required"`
	// Link is a site path the notification leads to; empty for none.
	Link      string
	State     NotificationState
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SetLink sets the site path the notification leads to.
func (n *Notification) SetLink(link string) { n.Link = link }

// SetUserId sets the notification recipient.
func (n *Notification) SetUserId(userId string) { n.UserId = userId }
