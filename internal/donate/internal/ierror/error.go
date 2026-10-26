package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

var (
	ErrNotFound            = ierror.NotFound("not found")
	ErrInsufficientFunds   = ierror.FailedPrecondition("insufficient funds")
	ErrAlreadyRefunded     = ierror.FailedPrecondition("purchase already refunded")
	ErrCannotIssueRefunded = ierror.FailedPrecondition("cannot mark refunded purchase as issued")
	// ErrShopCodeExists reports a rejection by the unique index on
	// donate_shop_items.code: an entry with this code is already in the shop.
	// Hidden entries count, so the caller can be looking at a code that the
	// public shop list does not show. The service answers AlreadyExists, which
	// the REST gateway surfaces as 409.
	ErrShopCodeExists = ierror.AlreadyExists("shop entry with this code already exists")
	// ErrItemTypeDisabled refuses a purchase of a legacy item-typed shop entry:
	// donate sells captured kits only, and the item branch of the compose step
	// is gone. Create/update already refuse the type, so this can only surface
	// for a legacy document that slipped past.
	ErrItemTypeDisabled = ierror.InvalidArgument("item-type shop entries are disabled: donate sells captured kits only")
)
