//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/donate/internal/repository/mongo

package repository

import (
	dto "github.com/lasthearth/vsservice/internal/donate/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/donate/internal/model"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
type Mapper interface {
	// goverter:ignore Model
	WalletToDTO(model.Wallet) dto.Wallet
}
