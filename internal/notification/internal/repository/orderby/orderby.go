package orderby

import (
	"fmt"
	"regexp"

	"go.mongodb.org/mongo-driver/bson"
)

type OrderByInfo struct {
	Field      string
	Direction  int
	MongoField string
}

// Regular expression for validating order_by according to AIP-132
// Format: "field_name asc|desc" or just "field_name" (default asc)
var orderByRegex = regexp.MustCompile(`^([a-z_]+)(?:\s+(asc|desc))?$`)

// allowedSortFields = map[string]string{
// 	"created_at": "created_at",
// 	"state":      "state",
// 	"title":      "title",
// }

func ParseOrderBy(orderBy string, allowedSortFields map[string]string) (*OrderByInfo, error) {
	// Если order_by пустой, используем значение по умолчанию
	if orderBy == "" {
		return &OrderByInfo{
			Field:      "created_at",
			Direction:  -1,
			MongoField: "created_at",
		}, nil
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

	sortDirection := 1
	if direction == "desc" {
		sortDirection = -1
	}

	return &OrderByInfo{
		Field:      fieldName,
		Direction:  sortDirection,
		MongoField: mongoField,
	}, nil
}

func BuildSortOptions(orderInfo *OrderByInfo) bson.D {
	sort := bson.D{}

	sort = append(sort, bson.E{Key: orderInfo.MongoField, Value: orderInfo.Direction})
	sort = append(sort, bson.E{Key: "_id", Value: -1})

	return sort
}
