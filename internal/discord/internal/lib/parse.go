package lib

import (
	"strings"

	"github.com/lasthearth/vsservice/internal/discord/internal/model"
)

// ParseMessageType extracts the chat message type from a prefix like [global].
func ParseMessageType(content string) (string, model.MessageType) {
	trimmed := strings.TrimSpace(content)

	if !strings.HasPrefix(trimmed, "[") {
		return trimmed, model.MessageTypeGlobal
	}

	end := strings.Index(trimmed, "]")
	if end == -1 {
		return trimmed, model.MessageTypeGlobal
	}

	prefix := strings.ToLower(trimmed[1:end])
	text := strings.TrimSpace(trimmed[end+1:])

	switch {
	case strings.Contains(prefix, "global"), strings.Contains(prefix, "глобал"):
		return text, model.MessageTypeGlobal
	case strings.Contains(prefix, "local"), strings.Contains(prefix, "локал"):
		return text, model.MessageTypeLocal
	case strings.Contains(prefix, "server"), strings.Contains(prefix, "сервер"):
		return text, model.MessageTypeServer
	case strings.Contains(prefix, "event"), strings.Contains(prefix, "событие"):
		return text, model.MessageTypeEvent
	default:
		return text, model.MessageTypeUnknown
	}
}
