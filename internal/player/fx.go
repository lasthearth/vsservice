package player

import (
	"github.com/lasthearth/vsservice/internal/player/internal/app/playerfx"
	"github.com/lasthearth/vsservice/internal/player/internal/app/verificationfx"
	"go.uber.org/fx"
)

var App = fx.Options(
	playerfx.App,
	verificationfx.App,
)
