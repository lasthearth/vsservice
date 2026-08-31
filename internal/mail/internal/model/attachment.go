package model

// Attachment is a single mail attachment: exactly one of Item or Coins is set.
// It is a value object (no constructor) so callers build it with a struct
// literal; the mapper layer round-trips it to proto/dto.
type Attachment struct {
	Item  *ItemAttachment
	Coins *CoinsAttachment
}

// ItemAttachment grants a game item or block. Validity of GameCode is the
// game's concern (checked at claim by VintageAPI), not vsservice's.
type ItemAttachment struct {
	GameCode     string
	Quantity     int32
	AttrSnapshot string // base64 TreeAttribute snapshot; empty = plain stack
	Type         string // "item" (default) or "block"
}

// CoinsAttachment grants donate-wallet coins.
type CoinsAttachment struct {
	Amount int64
}
