# Repository callback update

Reference for SKILL.md Step 5. For any read-modify-write of an existing document, expose a **callback** on the repository so the model stays the source of truth and mutation stays atomic.

## The callback shape

```go
UpdateX(ctx context.Context, id string,
	updateFn func(ctx context.Context, m *model.X) (*model.X, error),
) (*model.X, error)
```

Real example — `internal/settlement/internal/repository/mongo/repository.go` (`UpdateSettlement`, ~line 117):

```go
func (r *Repository) UpdateSettlement(
	ctx context.Context,
	id string,
	updateFn func(ctx context.Context, s *model.Settlement) (*model.Settlement, error),
) (*model.Settlement, error) {
	l := r.log.WithMethod("UpdateSettlement").With(zap.String("id", id))

	oid, err := mongomodel.ParseObjectID(id)
	if err != nil {
		return nil, repoerr.ErrNotFound
	}

	var d settlementdto.Settlement
	if err := r.setColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, repoerr.ErrNotFound
		}
		l.Error("failed to find settlement", zap.Error(err))
		return nil, err
	}

	m := r.mapper.FromSettlementDTO(d)        // dto  -> model
	updated, err := updateFn(ctx, &m)         // caller mutates the model via its methods
	if err != nil {
		return nil, err
	}

	updatedDTO := r.mapper.ToSettlementDTO(*updated) // model -> dto
	updatedDTO.Model = d.Model                       // preserve _id / timestamps from the stored doc
	updatedDTO.UpdatedAt = time.Now()

	if _, err := r.setColl.ReplaceOne(ctx, bson.M{"_id": oid}, updatedDTO); err != nil {
		l.Error("failed to replace settlement", zap.Error(err))
		return nil, err
	}
	return updated, nil
}
```

The caller (service) does the business logic inside the callback — calling model methods, never touching dto:

```go
updated, err := s.dbRepo.UpdateSettlement(ctx, id, func(ctx context.Context, m *model.Settlement) (*model.Settlement, error) {
	if err := m.SetDiplomacy(req.GetDiplomacy()); err != nil {
		return nil, err
	}
	return m, nil
})
```

Why this shape: the model enforces its own invariants (see `model-methods.md`), the repository handles only persistence + mapping, and the whole read-modify-write is one coherent unit. The service never sees a raw dto.

## When NOT to use the callback

Use plain mongo primitives directly only for narrow cases:

- **Field-set update** (a few known fields) — `UpdateOne` with `$set`. See `Update` in the same `repository.go` (~line 62).
- **Atomic counter / upsert** — `FindOneAndUpdate` with `$inc` / `$setOnInsert`, `SetUpsert(true)`, `SetReturnDocument(After)`. Real example — `internal/donate/internal/repository/mongo/wallet.go` `AddCoinsToWallet`:
  ```go
  update := bson.D{
	  {Key: "$inc", Value: bson.D{{Key: "coins", Value: amount}}},
	  {Key: "$set", Value: bson.D{{Key: "updated_at", Value: now}}},
	  {Key: "$setOnInsert", Value: bson.D{
		  {Key: "_id", Value: mongox.NewModel().Id},
		  {Key: "created_at", Value: now},
	  }},
  }
  opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
  ```

## Gotchas

- **Return typed `*DomainError`s, not raw errors.** `internal/pkg/ierror` is the shared typed-error base: `DomainError{Code codes.Code, Message string}` with constructors `NotFound / InvalidArgument / PermissionDenied / AlreadyExists / Internal / Unauthenticated / FailedPrecondition / ResourceExhausted`. The `DomainErrorUnaryInterceptor` (`internal/server/interceptor/domain_error.go`, wired in `internal/server/server.go`) auto-maps any `*DomainError` a handler returns to `status.Error(code, msg)` — so the `Errors:` block you wrote in the proto (see `proto.md`) stays honest for free. Don't hand-wrap `status.Error(...)` in the service.
- **Reuse the domain's existing error package + import alias.** Each domain declares named sentinels in `internal/<domain>/internal/ierror`, built on the `pkg/ierror` constructors — e.g. donate: `var ErrInsufficientFunds = ierror.FailedPrecondition("insufficient funds")`. Some domains alias the package on import (settlement: `repoerr "github.com/lasthearth/vsservice/internal/settlement/internal/ierror"` → `repoerr.ErrNotFound`); a few keep sentinels inside the repository dir instead (`internal/notification/.../repoerr/`, `internal/rules/.../mongo/errors.go`). Open the file you're editing and copy its import + alias verbatim. For a brand-new domain error, add a sentinel to the domain's `ierror` package on top of a `pkg/ierror` constructor — don't cross-import another domain's errors and don't invent a parallel package.
- `mgo` vs `mongo` is just an import alias for `go.mongodb.org/mongo-driver/v2/mongo` — donate aliases it `mgo` (`mgo.ErrNoDocuments`), settlement imports it as `mongo` (`mongo.ErrNoDocuments`). Identical; match the alias the file already uses.
- Always preserve the stored `Model` (`_id`, `created_at`) when round-tripping through dto; only bump `UpdatedAt`.
- Object IDs come from `mongox.NewModel().Id` on insert, `mongomodel.ParseObjectID(id)` on lookup.
