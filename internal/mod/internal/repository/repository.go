//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/mod/internal/repository
package repository

import (
	"context"
	"errors"
	"reflect"
	"time"

	dto "github.com/lasthearth/vsservice/internal/mod/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/mod/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
// goverter:extend AudienceToString
// goverter:extend AudienceFromString
// goverter:extend SourceToString
// goverter:extend SourceFromString
// goverter:extend SideToString
// goverter:extend SideFromString
type Mapper interface {
	// goverter:ignore Model
	FromModel(model.Mod) dto.Mod

	// goverter:autoMap Model
	ToModel(dto.Mod) model.Mod
}

// listSort is the display order. The id breaks ties, so a half-applied reorder
// cannot make two reads of the same list disagree.
var listSort = bson.D{{Key: "position", Value: 1}, {Key: "_id", Value: 1}}

// List returns every mod in display order, HIDDEN ones included.
func (r *Repository) List(ctx context.Context) ([]model.Mod, error) {
	return r.list(ctx, bson.M{})
}

// ListRequired returns the mods the "download everything" archive packs, in
// display order.
func (r *Repository) ListRequired(ctx context.Context) ([]model.Mod, error) {
	return r.list(ctx, bson.M{"audience": AudienceToString(model.AudienceRequired)})
}

func (r *Repository) list(ctx context.Context, filter bson.M) ([]model.Mod, error) {
	l := r.logger.WithMethod("List")

	cursor, err := r.coll.Find(ctx, filter, options.Find().SetSort(listSort))
	if err != nil {
		l.Error("failed to list mods", zap.Error(err))
		return nil, err
	}

	var docs []dto.Mod
	if err := cursor.All(ctx, &docs); err != nil {
		l.Error("failed to decode mods", zap.Error(err))
		return nil, err
	}

	mods := make([]model.Mod, 0, len(docs))
	for _, doc := range docs {
		mods = append(mods, *r.toModel(doc))
	}

	return mods, nil
}

// Get returns one mod by its ModDB id.
func (r *Repository) Get(ctx context.Context, modId string) (*model.Mod, error) {
	var doc dto.Mod
	if err := r.coll.FindOne(ctx, bson.M{"id": modId}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ierror.ErrNotInList
		}

		r.logger.WithMethod("Get").Error("failed to find mod", zap.String("id", modId), zap.Error(err))
		return nil, err
	}

	return r.toModel(doc), nil
}

// Create stores a new mod at the top of the list.
//
// A mod that is already there is refused by the unique index rather than by a
// read-then-write two admins could both pass, and the insert runs first so that
// the refusal leaves the stored order exactly as it was.
//
// The two writes are not a transaction. A failure between them leaves the new
// mod first and the others unshifted, so two mods share position 0 until a
// reorder renumbers the list; listSort breaks that tie by id, so the list still
// reads as one order rather than losing a mod.
func (r *Repository) Create(ctx context.Context, mod *model.Mod) error {
	doc := r.mapper.FromModel(*mod)
	doc.Model = mongox.NewModel()
	doc.Position = 0

	// The insert comes first, because it is what refuses a mod that is already
	// on the list, and a refusal must not leave the list changed. Shifting every
	// stored position first meant that a duplicate - a second click on the
	// panel's add button - moved the whole list down and put nothing at the top,
	// and that every retry moved it again.
	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ierror.ErrAlreadyExists
		}

		r.logger.WithMethod("Create").Error("failed to insert mod", zap.Error(err))
		return err
	}

	// Everyone else moves down one. The new mod is excluded: it is already
	// first. The envelope's version moves with the position, so a Save that read
	// the document before this write fails its guard instead of replacing the
	// whole document and putting the old position back.
	if _, err := r.coll.UpdateMany(
		ctx,
		bson.M{"id": bson.M{"$ne": mod.ModId}},
		bson.M{"$inc": bson.M{"position": 1, "version": 1}},
	); err != nil {
		r.logger.WithMethod("Create").Error("failed to shift positions", zap.Error(err))
		return err
	}

	return nil
}

// Save loads a mod, lets fn change it through its methods and stores it with a
// version-guarded replace (see mongox.UpdateDoc).
func (r *Repository) Save(
	ctx context.Context,
	modId string,
	fn func(ctx context.Context, mod *model.Mod) (*model.Mod, error),
) (*model.Mod, error) {
	updated, err := mongox.UpdateDoc(
		ctx,
		r.coll,
		bson.M{"id": modId},
		ierror.ErrNotInList,
		r.toModel,
		r.toDTO,
		fn,
	)
	if err != nil && !errors.Is(err, ierror.ErrNotInList) {
		r.logger.WithMethod("Save").Error("failed to update mod", zap.String("id", modId), zap.Error(err))
	}

	return updated, err
}

// Delete removes a mod from the list.
func (r *Repository) Delete(ctx context.Context, modId string) error {
	res, err := r.coll.DeleteOne(ctx, bson.M{"id": modId})
	if err != nil {
		r.logger.WithMethod("Delete").Error("failed to delete mod", zap.String("id", modId), zap.Error(err))
		return err
	}

	if res.DeletedCount == 0 {
		return ierror.ErrNotInList
	}

	return nil
}

// Reorder writes the display order. The caller has already checked that ids is
// a permutation of the stored mods, so this only applies it.
func (r *Repository) Reorder(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	writes := make([]mongo.WriteModel, 0, len(ids))
	for position, id := range ids {
		// The version moves with the position. mongox.UpdateDoc replaces a whole
		// document under a guard on the version it read, so a reorder that left
		// the version alone was invisible to a Save already in flight: the Save
		// matched, replaced the document, and wrote the position it had loaded
		// back, silently undoing the reorder.
		writes = append(writes, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"id": id}).
			SetUpdate(bson.M{
				"$set": bson.M{"position": position},
				"$inc": bson.M{"version": 1},
			}))
	}

	// Ordered: a bulk write that stops at the first failure leaves the front of
	// the list correct and the rest as it was, which listSort still reads as one
	// order. Unordered, it applies a scattered subset and the two ends disagree.
	if _, err := r.coll.BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(true)); err != nil {
		r.logger.WithMethod("Reorder").Error("failed to reorder mods", zap.Error(err))
		return err
	}

	return nil
}

// Sync stores the server-sourced part of the list in one pass: every mod in
// mods is written by id, a new one after the last position, and removeIds are
// deleted. A document whose id is neither written nor removed stays exactly as
// it is, which is what keeps the staff's OPTIONAL mods out of the sync's reach.
//
// The models arrive with the stored note and position already carried over, so
// the write never changes them. The envelope is taken from the document that is
// there: the sync must not reset the write time or the version guard a
// concurrent Save reads.
func (r *Repository) Sync(ctx context.Context, mods []model.Mod, removeIds []string) error {
	existing, err := r.storedByID(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Truncate(time.Millisecond)

	nextPosition := 0
	for _, doc := range existing {
		if doc.Position >= nextPosition {
			nextPosition = doc.Position + 1
		}
	}

	writes := make([]mongo.WriteModel, 0, len(mods)+1)

	for _, mod := range mods {
		doc := r.mapper.FromModel(mod)

		if old, ok := existing[mod.ModId]; ok {
			// An unchanged mod is not written again: the sync runs every
			// interval, and rewriting the same document would bump updated_at
			// (which the archive's timestamp reports) and the version guard a
			// concurrent Save reads, for no new data.
			if sameDocument(old, doc) {
				continue
			}

			doc.Model = mongox.Model{
				Id:        old.Id,
				CreatedAt: old.CreatedAt,
				UpdatedAt: now,
				Version:   old.Version + 1,
			}
		} else {
			doc.Model = mongox.NewModel()
			doc.Position = nextPosition
			nextPosition++
		}

		// Filtered by id alone rather than by a version the sync read: the sync
		// rewrites the same manifest every minute, so an upsert that loses a
		// race is retried on the next tick with the same result.
		writes = append(writes, mongo.NewReplaceOneModel().
			SetFilter(bson.M{"id": mod.ModId}).
			SetReplacement(doc).
			SetUpsert(true))
	}

	if len(removeIds) > 0 {
		writes = append(writes, mongo.NewDeleteManyModel().
			SetFilter(bson.M{"id": bson.M{"$in": removeIds}}))
	}

	if len(writes) == 0 {
		return nil
	}

	// Ordered: a write that stops leaves the list a prefix of the manifest, and
	// the next tick writes the rest, rather than a scattered subset.
	if _, err := r.coll.BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(true)); err != nil {
		r.logger.WithMethod("Sync").Error("failed to sync server mods", zap.Error(err))
		return err
	}

	return nil
}

// sameDocument reports whether two documents carry the same mod. The envelope
// (_id, timestamps, version) is not part of the comparison: the sync must skip
// the write when only those would change.
func sameDocument(a, b dto.Mod) bool {
	a.Model = mongox.Model{}
	b.Model = mongox.Model{}

	return reflect.DeepEqual(a, b)
}

// storedByID reads every stored document by its mod id. The sync needs the
// envelope as well as the fields, so it reads the documents rather than the
// domain models List returns.
func (r *Repository) storedByID(ctx context.Context) (map[string]dto.Mod, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		r.logger.WithMethod("Sync").Error("failed to list stored mods", zap.Error(err))
		return nil, err
	}

	var docs []dto.Mod
	if err := cursor.All(ctx, &docs); err != nil {
		r.logger.WithMethod("Sync").Error("failed to decode stored mods", zap.Error(err))
		return nil, err
	}

	byID := make(map[string]dto.Mod, len(docs))
	for _, doc := range docs {
		byID[doc.ModId] = doc
	}

	return byID, nil
}

// toModel maps a document into the domain model. The mapper carries the write
// time out of the Mongo envelope on its own.
func (r *Repository) toModel(doc dto.Mod) *model.Mod {
	mod := r.mapper.ToModel(doc)

	return &mod
}

// toDTO maps a model back to its document.
func (r *Repository) toDTO(mod *model.Mod) dto.Mod {
	return r.mapper.FromModel(*mod)
}
