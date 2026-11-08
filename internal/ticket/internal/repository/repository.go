//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/ticket/internal/repository
package repository

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/ticket/internal/dto"
	"github.com/lasthearth/vsservice/internal/ticket/internal/ierror"
	"github.com/lasthearth/vsservice/internal/ticket/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	// goverter:ignore Model
	FromModel(model.Ticket) dto.Ticket

	// goverter:autoMap Model
	ToModel(dto.Ticket) model.Ticket

	// goverter:ignore Model
	ReplyFromModel(model.SavedReply) dto.SavedReply

	// goverter:autoMap Model
	ReplyToModel(dto.SavedReply) model.SavedReply
}

// withoutThread leaves the messages out of list reads.
var withoutThread = bson.M{"messages": 0}

func (r *Repository) toModel(doc dto.Ticket) *model.Ticket {
	if doc.Messages == nil {
		doc.Messages = []dto.Message{}
	}
	m := r.mapper.ToModel(doc)
	return &m
}

func (r *Repository) toDTO(t *model.Ticket) dto.Ticket {
	return r.mapper.FromModel(*t)
}

// Create stores a new ticket.
func (r *Repository) Create(ctx context.Context, t *model.Ticket) (*model.Ticket, error) {
	doc := r.toDTO(t)
	doc.Model = mongox.NewModel()

	if _, err := r.tickets.InsertOne(ctx, doc); err != nil {
		r.logger.WithMethod("Create").Error("failed to insert ticket", zap.Error(err))
		return nil, err
	}
	return r.toModel(doc), nil
}

// Get returns a ticket with its thread.
func (r *Repository) Get(ctx context.Context, id string) (*model.Ticket, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var doc dto.Ticket
	if err := r.tickets.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		r.logger.WithMethod("Get").Error("failed to find ticket", zap.String("id", id), zap.Error(err))
		return nil, err
	}
	return r.toModel(doc), nil
}

// Update loads a ticket, lets fn change it through its methods and stores it
// with a version-guarded replace (see mongox.UpdateDoc).
func (r *Repository) Update(
	ctx context.Context,
	id string,
	fn func(ctx context.Context, t *model.Ticket) (*model.Ticket, error),
) (*model.Ticket, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	updated, err := mongox.UpdateDoc(ctx, r.tickets, bson.M{"_id": oid}, ierror.ErrNotFound, r.toModel, r.toDTO, fn)
	if err != nil && !isDomainErr(err) {
		r.logger.WithMethod("Update").Error("failed to update ticket", zap.String("id", id), zap.Error(err))
	}
	return updated, err
}

// CountActiveByAuthor counts the author's tickets that are not closed.
func (r *Repository) CountActiveByAuthor(ctx context.Context, authorId string) (int64, error) {
	n, err := r.tickets.CountDocuments(ctx, bson.M{
		"author_id": authorId,
		"status":    bson.M{"$ne": string(model.StatusClosed)},
	})
	if err != nil {
		r.logger.WithMethod("CountActiveByAuthor").Error("failed to count tickets", zap.Error(err))
	}
	return n, err
}

// CountUnreadByAuthor counts the author's tickets with a staff reply they
// have not read.
func (r *Repository) CountUnreadByAuthor(ctx context.Context, authorId string) (int64, error) {
	n, err := r.tickets.CountDocuments(ctx, bson.M{"author_id": authorId, "author_unread": true})
	if err != nil {
		r.logger.WithMethod("CountUnreadByAuthor").Error("failed to count tickets", zap.Error(err))
	}
	return n, err
}

// StaffCounters counts the staff inbox badges in one aggregation.
func (r *Repository) StaffCounters(ctx context.Context, staffId string) (model.StaffCounters, error) {
	isOpen := bson.M{"$eq": bson.A{"$status", string(model.StatusOpen)}}
	sumIf := func(cond any) bson.M {
		return bson.M{"$sum": bson.M{"$cond": bson.A{cond, 1, 0}}}
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"$or": bson.A{
			bson.M{"status": string(model.StatusOpen)},
			bson.M{"staff_unread": true},
		}}}},
		{{Key: "$group", Value: bson.M{
			"_id":  nil,
			"open": sumIf(isOpen),
			"open_unassigned": sumIf(bson.M{"$and": bson.A{
				isOpen, bson.M{"$eq": bson.A{"$assignee_id", ""}},
			}}),
			"open_mine": sumIf(bson.M{"$and": bson.A{
				isOpen, bson.M{"$eq": bson.A{"$assignee_id", staffId}},
			}}),
			"unread": sumIf(bson.M{"$eq": bson.A{"$staff_unread", true}}),
		}}},
	}

	cursor, err := r.tickets.Aggregate(ctx, pipeline)
	if err != nil {
		r.logger.WithMethod("StaffCounters").Error("failed to count tickets", zap.Error(err))
		return model.StaffCounters{}, err
	}
	var rows []struct {
		Open           int64 `bson:"open"`
		OpenUnassigned int64 `bson:"open_unassigned"`
		OpenMine       int64 `bson:"open_mine"`
		Unread         int64 `bson:"unread"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		r.logger.WithMethod("StaffCounters").Error("failed to decode counters", zap.Error(err))
		return model.StaffCounters{}, err
	}
	if len(rows) == 0 {
		return model.StaffCounters{}, nil
	}
	row := rows[0]
	return model.StaffCounters{
		Open:           row.Open,
		OpenUnassigned: row.OpenUnassigned,
		OpenMine:       row.OpenMine,
		Unread:         row.Unread,
	}, nil
}

// List returns one page of tickets without their threads and the token of the
// next page, empty on the last one.
func (r *Repository) List(ctx context.Context, q model.Query) ([]model.Ticket, string, error) {
	sortField := "last_message_at"
	filter := bson.M{}
	if q.Viewer == model.ViewerAuthor {
		sortField = "last_public_at"
	}
	if q.AuthorId != "" {
		filter["author_id"] = q.AuthorId
	}
	if q.Status != "" {
		filter["status"] = string(q.Status)
	}
	if q.Category != "" {
		filter["category"] = string(q.Category)
	}
	switch q.Assignee {
	case model.AssigneeMine:
		filter["assignee_id"] = q.StaffId
	case model.AssigneeNobody:
		filter["assignee_id"] = ""
	case model.AssigneeAny:
	}

	if q.PageToken != "" {
		at, oid, err := parsePageToken(q.PageToken)
		if err != nil {
			return nil, "", ierror.ErrBadPageToken
		}
		filter["$or"] = bson.A{
			bson.M{sortField: bson.M{"$lt": at}},
			bson.M{sortField: at, "_id": bson.M{"$lt": oid}},
		}
	}

	opts := options.Find().
		SetSort(bson.D{{Key: sortField, Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(q.Limit + 1)).
		SetProjection(withoutThread)

	cursor, err := r.tickets.Find(ctx, filter, opts)
	if err != nil {
		r.logger.WithMethod("List").Error("failed to list tickets", zap.Error(err))
		return nil, "", err
	}
	var docs []dto.Ticket
	if err := cursor.All(ctx, &docs); err != nil {
		r.logger.WithMethod("List").Error("failed to decode tickets", zap.Error(err))
		return nil, "", err
	}

	next := ""
	if len(docs) > q.Limit {
		docs = docs[:q.Limit]
		last := docs[len(docs)-1]
		at := last.LastMessageAt
		if q.Viewer == model.ViewerAuthor {
			at = last.LastPublicAt
		}
		next = pageToken(at, last.Id)
	}

	out := make([]model.Ticket, 0, len(docs))
	for _, doc := range docs {
		out = append(out, *r.toModel(doc))
	}
	return out, next, nil
}

// ListAutoCloseDue returns the ids of ANSWERED tickets answered before cutoff.
func (r *Repository) ListAutoCloseDue(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	opts := options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(int64(limit))
	cursor, err := r.tickets.Find(ctx, bson.M{
		"status":      string(model.StatusAnswered),
		"answered_at": bson.M{"$lte": cutoff},
	}, opts)
	if err != nil {
		r.logger.WithMethod("ListAutoCloseDue").Error("failed to find tickets", zap.Error(err))
		return nil, err
	}
	var docs []struct {
		Id bson.ObjectID `bson:"_id"`
	}
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(docs))
	for _, d := range docs {
		ids = append(ids, d.Id.Hex())
	}
	return ids, nil
}

// pageToken encodes the sort position of the last ticket on a page.
func pageToken(at time.Time, id bson.ObjectID) string {
	return strconv.FormatInt(at.UnixMilli(), 10) + "_" + id.Hex()
}

func parsePageToken(token string) (time.Time, bson.ObjectID, error) {
	ms, hex, ok := strings.Cut(token, "_")
	if !ok {
		return time.Time{}, bson.ObjectID{}, errors.New("no separator")
	}
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return time.Time{}, bson.ObjectID{}, err
	}
	oid, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		return time.Time{}, bson.ObjectID{}, err
	}
	return time.UnixMilli(n).UTC(), oid, nil
}

// isDomainErr reports outcomes the caller is told about and that are not worth
// an error log: model refusals, not-found, and losing the version guard under
// contention (mongox.ErrConflict) are all typed domain errors.
func isDomainErr(err error) bool {
	var de *pkgerr.DomainError
	return errors.As(err, &de)
}
