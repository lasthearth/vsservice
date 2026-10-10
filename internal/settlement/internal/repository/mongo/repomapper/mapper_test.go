//go:build !goverter

package repomapper

import (
	"testing"

	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	verificationdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/verification"
)

// A settlement re-created under a deleted one's id is built from the request
// document, which DeleteSettlement leaves behind. The request mirrors the
// notifier reissue counter, so the mapper must carry it: dropping it restarts
// the numbering at zero and the new settlement's reissue keys collide with the
// mails of the earlier life.
func TestFromVerificationCarriesTheNotifierCounter(t *testing.T) {
	var m MapperImpl

	dto := verificationdto.SettlementVerification{
		Name:             "Северный Оплот",
		Type:             "camp",
		Leader:           memberdto.Member{UserId: "owner1"},
		NotifierReissues: 3,
	}
	got := m.FromVerification(dto)

	if got.NotifierReissues != 3 {
		t.Errorf("NotifierReissues = %d, want 3", got.NotifierReissues)
	}
}

// A request that predates the mirrored counter has no such field, and decodes as
// zero: the first reissue of that settlement is number 1.
func TestFromVerificationDefaultsTheNotifierCounter(t *testing.T) {
	var m MapperImpl

	if got := m.FromVerification(verificationdto.SettlementVerification{}); got.NotifierReissues != 0 {
		t.Errorf("NotifierReissues = %d, want 0", got.NotifierReissues)
	}
}
