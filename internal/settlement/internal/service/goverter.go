package service

import (
	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

// TypeToProto converts a model.SettlementType to a settlementv1.SettlementType.
func TypeToProto(s model.SettlementType) settlementv1.SettlementType {
	var stype settlementv1.SettlementType

	switch s {
	case model.SettlementTypeProvince:
		stype = settlementv1.SettlementType_PROVINCE
	case model.SettlementTypeCity:
		stype = settlementv1.SettlementType_CITY
	case model.SettlementTypeVillage:
		stype = settlementv1.SettlementType_VILLAGE
	default:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_UNSPECIFIED
	}

	return stype
}
