package orderby

import (
	"fmt"
	"regexp"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Direction int

const (
	Asc  Direction = 1
	Desc Direction = -1
)

type OrderByInfo struct {
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
	defaultOrder *OrderByInfo,
) (*OrderByInfo, error) {
	if orderBy == "" {
		return defaultOrder, nil
	}

	if allowedSortFields == nil {
		return nil, fmt.Errorf("allowed sort fields is nil")
	}

	if len(allowedSortFields) == 0 {
		return nil, fmt.Errorf("allowed sort fields is empty")
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

	return &OrderByInfo{
		Field:      fieldName,
		Direction:  sortDirection,
		MongoField: mongoField,
	}, nil
}

func BuildSortOptions(orderInfo *OrderByInfo) bson.D {
	sort := bson.D{}
	sort = slices.Insert(sort, 0, bson.E{Key: "_id", Value: Desc})
	if orderInfo == nil {
		return sort
	}

	sort = append(sort, bson.E{Key: orderInfo.MongoField, Value: orderInfo.Direction})

	return sort
}
