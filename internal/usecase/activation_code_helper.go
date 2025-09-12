package usecase

import (
	"crypto/rand"
	"io"
)

// generateActivationCode creates a secure, random, and human-readable activation code.
// Format: XXXX-XXXX-XXXX
func generateActivationCode() (string, error) {
	// A character set that avoids ambiguous characters like O/0, I/1, l.
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	const codeLength = 12

	buffer := make([]byte, codeLength)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return "", err
	}

	for i := 0; i < codeLength; i++ {
		buffer[i] = chars[int(buffer[i])%len(chars)]
	}

	// Format as XXXX-XXXX-XXXX
	return string(buffer[0:4]) + "-" + string(buffer[4:8]) + "-" + string(buffer[8:12]), nil
}
