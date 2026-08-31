package goverter

import (
	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
)

// AttachmentModelToProto converts a domain Attachment (exactly one of Item or
// Coins) to its proto oneof form.
func AttachmentModelToProto(a model.Attachment) *mailv1.Attachment {
	switch {
	case a.Item != nil:
		return &mailv1.Attachment{Kind: &mailv1.Attachment_Item{Item: &mailv1.ItemAttachment{
			GameCode:     a.Item.GameCode,
			Quantity:     a.Item.Quantity,
			AttrSnapshot: a.Item.AttrSnapshot,
			Type:         a.Item.Type,
		}}}
	case a.Coins != nil:
		return &mailv1.Attachment{Kind: &mailv1.Attachment_Coins{Coins: &mailv1.CoinsAttachment{
			Amount: a.Coins.Amount,
		}}}
	default:
		return &mailv1.Attachment{}
	}
}

// MailStateToProto maps the domain state string to its proto enum.
func MailStateToProto(s model.MailState) mailv1.MailState {
	switch s {
	case model.MailStateUnread:
		return mailv1.MailState_MAIL_STATE_UNREAD
	case model.MailStateRead:
		return mailv1.MailState_MAIL_STATE_READ
	case model.MailStateClaimed:
		return mailv1.MailState_MAIL_STATE_CLAIMED
	case model.MailStateExpired:
		return mailv1.MailState_MAIL_STATE_EXPIRED
	case model.MailStateRevoked:
		return mailv1.MailState_MAIL_STATE_REVOKED
	default:
		return mailv1.MailState_MAIL_STATE_UNSPECIFIED
	}
}

// AttachmentProtoToModel converts a proto Attachment to the domain form. Used
// by the service when persisting a composed mail.
func AttachmentProtoToModel(a *mailv1.Attachment) model.Attachment {
	if item := a.GetItem(); item != nil {
		return model.Attachment{Item: &model.ItemAttachment{
			GameCode:     item.GetGameCode(),
			Quantity:     item.GetQuantity(),
			AttrSnapshot: item.GetAttrSnapshot(),
			Type:         item.GetType(),
		}}
	}
	if coins := a.GetCoins(); coins != nil {
		return model.Attachment{Coins: &model.CoinsAttachment{Amount: coins.GetAmount()}}
	}
	return model.Attachment{}
}

// AttachmentsProtoToModel converts a proto attachment slice to the domain form.
func AttachmentsProtoToModel(as []*mailv1.Attachment) []model.Attachment {
	if as == nil {
		return nil
	}
	out := make([]model.Attachment, len(as))
	for i, a := range as {
		out[i] = AttachmentProtoToModel(a)
	}
	return out
}
