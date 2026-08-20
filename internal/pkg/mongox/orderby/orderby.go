package orderby

import (
	"errors"
	"fmt"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Direction int

const (
	Asc  Direction = 1
	Desc Direction = -1
)

type Info struct {
	Field      string
	Direction  Direction
	MongoField string
}

// Regular expression for validating order_by according to AIP-132
// Format: "field_name asc|desc" or just "field_name" (default asc)
var orderByRegex = regexp.MustCompile(`^([a-z_]+)(?:\s+(asc|desc))?$`)

func Parse(
	orderBy string,
	allowedSortFields map[string]string,
	defaultOrder *Info,
) (*Info, error) {
	if orderBy == "" {
		return defaultOrder, nil
	}

	if allowedSortFields == nil {
		return nil, errors.New("allowed sort fields is nil")
	}

	if len(allowedSortFields) == 0 {
		return nil, errors.New("allowed sort fields is empty")
	}

	matches := orderByRegex.FindStringSubmatch(orderBy)
	if len(matches) == 0 {
		return nil, fmt.Errorf("invalid order_by format: %s. Expected format: 'field_name [asc|desc]'", orderBy)
	}

	fieldName := matches[1]
	direction := matches[2]

	mongoField, allowed := allowedSortFields[fieldName]
	if !allowed {
		return nil, fmt.Errorf("sorting by field '%s' is not allowed", fieldName)
	}

	sortDirection := Asc
	if direction == "desc" {
		sortDirection = Desc
	}

	return &Info{
		Field:      fieldName,
		Direction:  sortDirection,
		MongoField: mongoField,
	}, nil
}

// BuildSortOptions returns the caller's field as the primary sort key.
// `_id` is appended last as the deterministic tiebreaker cursor pagination
// needs — pagination.List adds it when absent, so it is only spelled out here
// for callers that pass the sort elsewhere.
func BuildSortOptions(orderInfo *Info) bson.D {
	if orderInfo == nil {
		return bson.D{{Key: "_id", Value: Desc}}
	}

	return bson.D{
		{Key: orderInfo.MongoField, Value: orderInfo.Direction},
		{Key: "_id", Value: orderInfo.Direction},
	}
}
