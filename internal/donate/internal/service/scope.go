package service

import "github.com/lasthearth/vsservice/internal/server/interceptor"

func (s *Service) Scope() map[interceptor.Method]interceptor.Scope {
	srvName := "/donate.v1.DonateService/"
	return map[interceptor.Method]interceptor.Scope{
		interceptor.Method(srvName + "AddCoins"):                  interceptor.Scope("donate:coins:add"),
		interceptor.Method(srvName + "DeductCoins"):               interceptor.Scope("donate:coins:deduct"),
		interceptor.Method(srvName + "CreateShopItem"):            interceptor.Scope("donate:shop:create"),
		interceptor.Method(srvName + "UpdateShopItem"):            interceptor.Scope("donate:shop:update"),
		interceptor.Method(srvName + "DeleteShopItem"):            interceptor.Scope("donate:shop:delete"),
		interceptor.Method(srvName + "Refund"):                    interceptor.Scope("donate:purchase:refund"),
		interceptor.Method(srvName + "AdminGetPlayerBalance"):     interceptor.Scope("donate:wallet:read"),
		interceptor.Method(srvName + "ListTransactions"):          interceptor.Scope("donate:transaction:list"),
		interceptor.Method(srvName + "AdminListPurchases"):        interceptor.Scope("donate:purchase:list"),
		interceptor.Method(srvName + "AdminListAllPurchases"):     interceptor.Scope("donate:purchase:list"),
		interceptor.Method(srvName + "AdminListPendingPurchases"): interceptor.Scope("donate:purchase:list"),
		interceptor.Method(srvName + "MarkPurchaseIssued"):        interceptor.Scope("donate:purchase:issue"),
		interceptor.Method(srvName + "ListWallets"):               interceptor.Scope("donate:wallet:read"),
	}
}
