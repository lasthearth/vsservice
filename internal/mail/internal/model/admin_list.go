package model

import "time"

// AdminMailFilter narrows the admin mail list. BroadcastOnly wins over
// Recipient when both are set: "only broadcasts" cannot also mean "only
// broadcast" plus some other recipient.
type AdminMailFilter struct {
	Recipient     string
	BroadcastOnly bool
}

// NewAdminMailFilter builds the filter from the request fields.
func NewAdminMailFilter(recipient string, broadcastOnly bool) AdminMailFilter {
	return AdminMailFilter{Recipient: recipient, BroadcastOnly: broadcastOnly}
}

// AdminMailEntry is one row of the admin mail list: a mail flattened for
// reading plus the number of players who claimed it. ClaimedCount is derived at
// read from the mail_claims collection and never lives on the Mail document.
type AdminMailEntry struct {
	Id           string
	Recipient    string
	Broadcast    bool
	Sender       string
	Title        string
	Body         string
	Attachments  []Attachment
	CreatedAt    time.Time
	ExpiresAt    *time.Time
	Revoked      bool
	ClaimedCount int64
}

// NewAdminMailEntry flattens a mail and its claim count into a list row.
func NewAdminMailEntry(m *Mail, claimedCount int64) *AdminMailEntry {
	return &AdminMailEntry{
		Id:           m.Id,
		Recipient:    m.Recipient,
		Broadcast:    m.Recipient == RecipientBroadcast,
		Sender:       m.Sender,
		Title:        m.Title,
		Body:         m.Body,
		Attachments:  m.Attachments,
		CreatedAt:    m.CreatedAt,
		ExpiresAt:    m.ExpiresAt,
		Revoked:      m.Revoked,
		ClaimedCount: claimedCount,
	}
}
