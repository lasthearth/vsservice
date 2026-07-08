# Mapping with goverter

Reference for SKILL.md Step 3. All type conversion in vsservice goes through goverter-generated mappers — never hand-write conversion between proto, model, and dto.

## Two mappers per domain

- **`service.Mapper`** — domain `model` ↔ protobuf. Interface lives in the service package; output `sermapper/mapper.go`.
- **`repository.Mapper`** — domain `model` ↔ mongo `dto`. Interface lives in the repository package; output `repomapper/mapper.go`.

## The generate directive + converter interface

The interface file carries the `//go:generate` line and the `// goverter:*` config as comments directly above the interface. Real example — `internal/settlement/internal/service/interface.go`:

```go
//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/settlement/internal/service
package service

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend TypeToProto
// goverter:extend TagIdsToProto
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:IntToInt32
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToSettlementProto(model.Settlement) *settlementv1.Settlement
	ToSettlementProtos([]model.Settlement) []*settlementv1.Settlement
}
```

Repository-side mirror — `internal/settlement/internal/repository/mongo/app.go`:

```go
//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo
package repository

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToObjectId
type Mapper interface {
	FromSettlementDTO(dto settlementdto.Settlement) model.Settlement
	ToSettlementDTO(model.Settlement) settlementdto.Settlement
}
```

Notes:
- Shared extend helpers live in `internal/pkg/goverter` (`TimeToTimestamp`, `TimeToInt64`, `IntToInt32`, `ObjectIdToString`, …). Reach for these before writing a domain-local extend.
- `// goverter:ignore state sizeCache unknownFields` on every proto-returning method (protobuf internal fields).
- Regenerate with `make generate` (runs `go generate ./...`).

## Provide the mapper in the OUTER fx.go

This is the easy-to-miss part. The generated type is `*sermapper.MapperImpl` / `*repomapper.MapperImpl`. Construct and bind it in the domain's outer `internal/<domain>/fx.go`, **not** inside the package that holds the interface:

```go
fx.Provide(
	fx.Private,
	fx.Annotate(func() *repomapper.MapperImpl { return &repomapper.MapperImpl{} },
		fx.As(new(repository.Mapper))),
	fx.Annotate(func() *sermapper.MapperImpl { return &sermapper.MapperImpl{} },
		fx.As(new(service.Mapper))),
),
```

Why outer: `//go:generate` runs `goverter gen <import-path>`. If the converter interface and its `MapperImpl` live in the same package, goverter cannot generate the impl into that package without a cycle, and `go generate` / the build fails. Keeping the interface in `service`/`repository` and the generated `MapperImpl` in a sibling `sermapper`/`repomapper` package — then wiring from the outer `fx.go` — is what makes it compile.

## Inject into the service

The `Mapper` interface is a field on the service's `Opts` (`fx.In`) — `internal/settlement/internal/service/app.go`:

```go
type Opts struct {
	fx.In
	DbRepo   SettlementRepository
	Mapper   Mapper
	Storage  Storage
	// …
}
```

fx resolves `Mapper` to the `*sermapper.MapperImpl` bound above. Store it on the service struct and call `s.mapper.ToSettlementProto(...)` etc.
