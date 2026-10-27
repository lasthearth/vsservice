package logger

import (
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// sensitiveFragments matches a lowercased field key by substring, so
// access_token, client_secret, Authorization and mongo_url are all covered by
// one table. A hit means the value never reaches the output.
var sensitiveFragments = []string{
	"token",
	"secret",
	"password",
	"authorization",
	"private_key",
	"apikey",
	"api_key",
	"credential",
	"connection_string",
	"mongo_url",
	"jwks",
	"cookie",
}

func keyIsSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, frag := range sensitiveFragments {
		if strings.Contains(k, frag) {
			return true
		}
	}
	return false
}

// redactedValue replaces any sensitive field value regardless of its type.
const redactedValue = "[redacted]"

// redactingCore wraps a zap core and masks sensitive field values on the way
// out. Wrapped in logger.New, so every derived logger (Named, With) inherits
// the masking.
type redactingCore struct {
	zapcore.Core
}

func (c redactingCore) With(fields []zap.Field) zapcore.Core {
	return redactingCore{Core: c.Core.With(redactFields(fields))}
}

func (c redactingCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Core.Enabled(ent.Level) {
		// AddCore routes the write through our Write, not the inner core's.
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c redactingCore) Write(ent zapcore.Entry, fields []zap.Field) error {
	return c.Core.Write(ent, redactFields(fields))
}

func (c redactingCore) Sync() error {
	return c.Core.Sync()
}

// redactFields returns a copy with sensitive values masked. zap.Error fields
// keep their key ("error") and their value: golang-jwt and driver errors do
// not embed the credential itself, and the error text is the debugging signal.
func redactFields(fields []zap.Field) []zap.Field {
	out := make([]zap.Field, len(fields))
	copy(out, fields)
	for i, f := range out {
		if !keyIsSensitive(f.Key) {
			continue
		}
		out[i] = zap.Field{
			Type:   zapcore.StringType,
			Key:    f.Key,
			String: redactedValue,
		}
	}
	return out
}
