package model

// MessageType represents the category of a Discord game-chat message
// derived from its [prefix].
type MessageType string

const (
	MessageTypeGlobal  MessageType = "global"
	MessageTypeLocal   MessageType = "local"
	MessageTypeServer  MessageType = "server"
	MessageTypeEvent   MessageType = "event"
	MessageTypeUnknown MessageType = "unknown"
)

// Message is a cleaned, UI-ready Discord message.
type Message struct {
	ID        string
	Content   string
	Author    string
	Timestamp string
	Type      MessageType
}

// Image is a UI-ready image attachment extracted from Discord messages.
type Image struct {
	ID        string
	URL       string
	ProxyURL  string
	Alt       string
	Author    string
	Timestamp string
	Width     int32
	Height    int32
}
