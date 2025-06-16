package service

import (
	"fmt"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

// TypeToProto converts a model.SettlementType to a settlementv1.SettlementType.
func TypeToProto(s model.SettlementType) settlementv1.SettlementType {
	var stype settlementv1.SettlementType

	switch s {
	case model.SettlementTypeCamp:
		stype = settlementv1.SettlementType_CAMP
	case model.SettlementTypeCity:
		stype = settlementv1.SettlementType_CITY
	case model.SettlementTypeGuild:
		stype = settlementv1.SettlementType_GUILD
	case model.SettlementTypeGuildLvl2:
		stype = settlementv1.SettlementType_GUILD_LVL2
	case model.SettlementTypeOrden:
		stype = settlementv1.SettlementType_ORDEN
	case model.SettlementTypeProvince:
		stype = settlementv1.SettlementType_PROVINCE
	case model.SettlementTypeVillage:
		stype = settlementv1.SettlementType_VILLAGE
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
	case settlementv1.SettlementType_CITY:
		s = model.SettlementTypeCity
	case settlementv1.SettlementType_GUILD:
		s = model.SettlementTypeGuild
	case settlementv1.SettlementType_GUILD_LVL2:
		s = model.SettlementTypeGuildLvl2
	case settlementv1.SettlementType_ORDEN:
		s = model.SettlementTypeOrden
	case settlementv1.SettlementType_PROVINCE:
		s = model.SettlementTypeProvince
	case settlementv1.SettlementType_VILLAGE:
		s = model.SettlementTypeVillage
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
	case settlementv1.SubmitRequest_GUILD:
		s = model.SettlementTypeGuild
	case settlementv1.SubmitRequest_ORDEN:
		s = model.SettlementTypeOrden
	default:
		return nil, fmt.Errorf("unknown settlement type: %v", req)
	}
	return &s, nil
}
