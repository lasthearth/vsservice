package code

import (
	"crypto/rand"
	"encoding/hex"
)

func Generate() string {
	b := make([]byte, 3)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
