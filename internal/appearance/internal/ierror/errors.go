package ierror

import "github.com/lasthearth/vsservice/internal/pkg/ierror"

// Unknown and Locked carry the model's message, which names the item.
func Unknown(msg string) error { return ierror.InvalidArgument(msg) }

func Locked(msg string) error { return ierror.FailedPrecondition(msg) }

// Owned: the item was bought already.
func Owned(msg string) error { return ierror.AlreadyExists(msg) }

// NoFunds: the wallet cannot pay for the item.
var NoFunds = ierror.FailedPrecondition("not enough shards")
