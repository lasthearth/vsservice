package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
)

// Repository is the single persistence contract for the hungergames domain.
// The implementation lives in internal/hungergames/internal/repository/mongo.
type Repository interface {
	// Player stats (current season)

	// GetPlayerStats returns stats for a player in the given season.
	// Returns ierror.ErrNotFound if no record exists.
	GetPlayerStats(ctx context.Context, seasonID, playerID string) (*model.PlayerStats, error)

	// GetPlayerStatsByIDs returns stats for the given player IDs in the given season.
	// Missing players are silently omitted from the result.
	GetPlayerStatsByIDs(ctx context.Context, seasonID string, playerIDs []string) ([]*model.PlayerStats, error)

	// SavePlayerStats upserts a player stats document (matched by PlayerID + SeasonID).
	SavePlayerStats(ctx context.Context, stats *model.PlayerStats) error

	// ListPlayerStatsByELO returns the top `limit` players in the current season
	// ordered by ELO descending.
	ListPlayerStatsByELO(ctx context.Context, limit int) ([]*model.PlayerStats, error)

	// ListAllPlayerStatsByELO returns all players in the current season
	// ordered by ELO descending. Used during season reset.
	ListAllPlayerStatsByELO(ctx context.Context) ([]*model.PlayerStats, error)

	// DeleteAllPlayerStats removes all current-season player stats (hard reset).
	DeleteAllPlayerStats(ctx context.Context) error

	// Seasons

	// GetActiveSeason returns the currently open season.
	// Returns ierror.ErrNoActiveSeason if none exists.
	GetActiveSeason(ctx context.Context) (*model.Season, error)

	// GetSeasonByID returns a season by its ID.
	// Returns ierror.ErrNotFound if not found.
	GetSeasonByID(ctx context.Context, id string) (*model.Season, error)

	// CreateSeason inserts a new season and returns the created record.
	CreateSeason(ctx context.Context, season *model.Season) (*model.Season, error)

	// CloseSeason sets ended_at on the season with the given ID.
	CloseSeason(ctx context.Context, id string) error

	// CountSeasons returns the total number of seasons (for numbering the next one).
	CountSeasons(ctx context.Context) (int, error)

	// ListSeasons returns a paginated list of seasons ordered by number descending.
	ListSeasons(ctx context.Context, next string, limit int) ([]*model.Season, string, error)

	// Season results

	// CreateSeasonResults inserts the archived standings for a completed season.
	CreateSeasonResults(ctx context.Context, results []*model.SeasonResult) error

	// ListSeasonResults returns all archived results for a season, ordered by rank.
	ListSeasonResults(ctx context.Context, seasonID string) ([]*model.SeasonResult, error)

	// GetPlayerSeasonResult returns a single player's archived result for a season.
	// Returns ierror.ErrNotFound if not found.
	GetPlayerSeasonResult(ctx context.Context, seasonID, playerID string) (*model.SeasonResult, error)

	// Coins (direct writes to donate domain collections)

	// AddCoinsToWallet atomically upserts the donate wallet and increments coins.
	AddCoinsToWallet(ctx context.Context, playerID, playerName string, amount int64) error

	// CreateCreditTransaction records a credit entry in the donate transactions log.
	CreateCreditTransaction(ctx context.Context, playerID string, amount int64, reason string) error
}
