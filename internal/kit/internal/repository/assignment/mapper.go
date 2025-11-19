//go:generate goverter gen github.com/lasthearth/vsservice/internal/kit/internal/repository/assignment
package assignment

import (
	dto "github.com/lasthearth/vsservice/internal/kit/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/kit/internal/model"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
type Mapper interface {
	// goverter:autoMap Model
	ToAssignment(dto dto.Assignment) model.KitAssignment
	ToAssignments(dtos []dto.Assignment) []model.KitAssignment

	// goverter:ignore Model
	FromAssignment(assignment model.KitAssignment) dto.Assignment
	FromAssignments(assignments []model.KitAssignment) []dto.Assignment
}
