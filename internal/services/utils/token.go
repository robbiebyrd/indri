package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateToken returns a cryptographically random 256-bit token, hex-encoded.
// It is used as an unguessable session bearer token, in place of the
// database ObjectID, so a session cannot be resumed by enumerating IDs.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session token: %w", err)
	}

	return hex.EncodeToString(b), nil
}
