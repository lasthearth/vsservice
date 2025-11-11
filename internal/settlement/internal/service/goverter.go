package service

import (
	"fmt"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

func TagIdsToProto(ids []string) []*settlementv1.TagReference {
	var tagIds []*settlementv1.TagReference
	for _, id := range ids {
		tagIds = append(tagIds, &settlementv1.TagReference{
			Id: id,
		})
	}
	return tagIds
}

// TypeToProto converts a model.SettlementType to a settlementv1.SettlementType.
func TypeToProto(s model.SettlementType) settlementv1.SettlementType {
	var stype settlementv1.SettlementType

	switch s {
	case model.SettlementTypeCamp:
		stype = settlementv1.SettlementType_CAMP
	case model.SettlementTypeVillage:
		stype = settlementv1.SettlementType_VILLAGE
	case model.SettlementTypeTownship:
		stype = settlementv1.SettlementType_TOWNSHIP
	case model.SettlementTypeCity:
		stype = settlementv1.SettlementType_CITY
	case model.SettlementTypeProvince:
		stype = settlementv1.SettlementType_PROVINCE
	default:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_UNSPECIFIED
	}
	return stype
}

func TypeFromProto(stype settlementv1.SettlementType) (*model.SettlementType, error) {
	var s model.SettlementType

	switch stype {
	case settlementv1.SettlementType_CAMP:
		s = model.SettlementTypeCamp
	case settlementv1.SettlementType_VILLAGE:
		s = model.SettlementTypeVillage
	case settlementv1.SettlementType_TOWNSHIP:
		s = model.SettlementTypeTownship
	case settlementv1.SettlementType_CITY:
		s = model.SettlementTypeCity
	case settlementv1.SettlementType_PROVINCE:
		s = model.SettlementTypeProvince
	default:
		return nil, fmt.Errorf("unknown settlement type: %v", stype)
	}
	return &s, nil
}

// TypeFromReqProto converts a SubmitRequest_Type to a SettlementType.
func TypeFromReqProto(req settlementv1.SubmitRequest_Type) (*model.SettlementType, error) {
	var s model.SettlementType

	switch req {
	case settlementv1.SubmitRequest_CAMP:
		s = model.SettlementTypeCamp
	default:
		return nil, fmt.Errorf("unknown settlement type: %v", req)
	}
	return &s, nil
}
