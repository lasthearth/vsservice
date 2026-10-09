//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/appearance/internal/service
package service

import (
	"context"
	"time"

	appearancev1 "github.com/lasthearth/vsservice/gen/appearance/v1"
	"github.com/lasthearth/vsservice/internal/appearance/internal/model"
	"github.com/lasthearth/vsservice/internal/appearance/internal/repository"
)

// goverter:converter
// goverter:output:file sermapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTimestamp
type Mapper interface {
	// goverter:ignore state sizeCache unknownFields
	ToProto(model.Appearance) *appearancev1.Appearance
	ToProtos([]model.Appearance) []*appearancev1.Appearance

	// goverter:useZeroValueOnPointerInconsistency
	RequestToChoice(*appearancev1.UpdateMyAppearanceRequest) model.Choice
}

var (
	_ Repository     = (*repository.Repository)(nil)
	_ StandingReader = (*repository.Standings)(nil)
)

// Repository stores appearances.
type Repository interface {
	List(ctx context.Context) ([]model.Appearance, error)
	Save(ctx context.Context, a *model.Appearance) error
	Delete(ctx context.Context, userID string) error
	AddPurchase(ctx context.Context, userID, itemID string, price int64, at time.Time) error
}

// Wallet pays for items bought with shards (donate's public use case).
type Wallet interface {
	Debit(ctx context.Context, playerID string, amount int64, reason string) error
	Credit(ctx context.Context, playerID, playerName string, amount int64, reason string) error
}

// StandingReader tells what the player has unlocked.
type StandingReader interface {
	Standing(ctx context.Context, userID string) (model.Standing, error)
}
