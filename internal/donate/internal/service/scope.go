package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/donate.v1.DonateService/"
	manage := interceptor.Scope("donate:manage")
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "AddCoins"):          manage,
		interceptor.Method(srvName + "DeductCoins"):       manage,
		interceptor.Method(srvName + "CreateShopItem"):    manage,
		interceptor.Method(srvName + "UpdateShopItem"):    manage,
		interceptor.Method(srvName + "DeleteShopItem"):    manage,
		interceptor.Method(srvName + "Refund"):            manage,
		interceptor.Method(srvName + "ListTransactions"):  manage,
		interceptor.Method(srvName + "AdminListPurchases"): manage,
		interceptor.Method(srvName + "AdminListPendingPurchases"): manage,
		interceptor.Method(srvName + "MarkPurchaseIssued"):        manage,
	}
}
