package repository

import (
	"context"
	"errors"

	dto "github.com/lasthearth/vsservice/internal/mail/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/mail/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) CreateMail(ctx context.Context, mail *model.Mail) (*model.Mail, error) {
	// Idempotency: a repeat with the same key returns the existing mail.
	if mail.IdempotencyKey != "" {
		var existing dto.Mail
		err := r.mailColl.FindOne(ctx, bson.M{"idempotency_key": mail.IdempotencyKey}).Decode(&existing)
		if err == nil {
			return mailFromDTO(existing), nil
		}
		if !errors.Is(err, mgo.ErrNoDocuments) {
			return nil, err
		}
	}

	m := mongox.NewModel()
	d := mailToDTO(mail)
	d.Model = m

	result, err := r.mailColl.InsertOne(ctx, d)
	if err != nil {
		// A racing insert on the same key: fetch and return the winner.
		if mail.IdempotencyKey != "" && mgo.IsDuplicateKeyError(err) {
			var existing dto.Mail
			if ferr := r.mailColl.FindOne(ctx, bson.M{"idempotency_key": mail.IdempotencyKey}).Decode(&existing); ferr == nil {
				return mailFromDTO(existing), nil
			}
		}
		return nil, err
	}

	oid, err := mongox.ParseAnyObjectID(result.InsertedID)
	if err != nil {
		return nil, err
	}
	mail.MarkCreated(oid.Hex(), m.CreatedAt)
	return mail, nil
}

func (r *Repository) GetMail(ctx context.Context, id string) (*model.Mail, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var d dto.Mail
	if err := r.mailColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		if errors.Is(err, mgo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		return nil, err
	}
	return mailFromDTO(d), nil
}

func (r *Repository) UpdateMail(
	ctx context.Context,
	id string,
	updateFn func(ctx context.Context, m *model.Mail) (*model.Mail, error),
) (*model.Mail, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	return mongox.UpdateDoc(
		ctx,
		r.mailColl,
		bson.M{"_id": oid},
		ierror.ErrNotFound,
		mailFromDTO,
		mailToDTOPtr,
		updateFn,
	)
}

func (r *Repository) ListMailsForRecipient(ctx context.Context, playerID string) ([]*model.Mail, error) {
	l := r.log.With(zap.String("method", "ListMailsForRecipient"), zap.String("player_id", playerID))

	filter := bson.M{"recipient": bson.M{"$in": bson.A{playerID, model.RecipientBroadcast}}}
	cursor, err := r.mailColl.Find(ctx, filter)
	if err != nil {
		l.Error("failed to find mails", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []dto.Mail
	if err := cursor.All(ctx, &dtos); err != nil {
		l.Error("failed to decode mails", zap.Error(err))
		return nil, err
	}

	result := make([]*model.Mail, len(dtos))
	for i, d := range dtos {
		result[i] = mailFromDTO(d)
	}
	return result, nil
}

func mailFromDTO(d dto.Mail) *model.Mail {
	return model.ReconstituteMail(
		d.Model.Id.Hex(),
		d.Recipient, d.Sender, d.Title, d.Body,
		attachmentsFromDTO(d.Attachments),
		d.CreatedAt, d.ExpiresAt, d.Revoked, d.IdempotencyKey,
	)
}

// mailToDTO builds a BSON-ready Mail DTO. The mongox.Model envelope is owned by
// the caller (mongox.UpdateDoc / CreateMail), not by this conversion.
func mailToDTO(m *model.Mail) dto.Mail {
	return dto.Mail{
		Recipient:      m.Recipient,
		Sender:         m.Sender,
		Title:          m.Title,
		Body:           m.Body,
		Attachments:    attachmentsToDTO(m.Attachments),
		ExpiresAt:      m.ExpiresAt,
		Revoked:        m.Revoked,
		IdempotencyKey: m.IdempotencyKey,
	}
}

// mailToDTOPtr adapts mailToDTO to the *model signature UpdateDoc expects.
func mailToDTOPtr(m *model.Mail) dto.Mail { return mailToDTO(m) }

func attachmentsFromDTO(as []dto.Attachment) []model.Attachment {
	if as == nil {
		return nil
	}
	out := make([]model.Attachment, len(as))
	for i, a := range as {
		switch {
		case a.Item != nil:
			out[i] = model.Attachment{Item: &model.ItemAttachment{
				GameCode:     a.Item.GameCode,
				Quantity:     a.Item.Quantity,
				AttrSnapshot: a.Item.AttrSnapshot,
				Type:         a.Item.Type,
			}}
		case a.Coins != nil:
			out[i] = model.Attachment{Coins: &model.CoinsAttachment{Amount: a.Coins.Amount}}
		}
	}
	return out
}

func attachmentsToDTO(as []model.Attachment) []dto.Attachment {
	if as == nil {
		return nil
	}
	out := make([]dto.Attachment, len(as))
	for i, a := range as {
		switch {
		case a.Item != nil:
			out[i] = dto.Attachment{Item: &dto.ItemAttachment{
				GameCode:     a.Item.GameCode,
				Quantity:     a.Item.Quantity,
				AttrSnapshot: a.Item.AttrSnapshot,
				Type:         a.Item.Type,
			}}
		case a.Coins != nil:
			out[i] = dto.Attachment{Coins: &dto.CoinsAttachment{Amount: a.Coins.Amount}}
		}
	}
	return out
}
