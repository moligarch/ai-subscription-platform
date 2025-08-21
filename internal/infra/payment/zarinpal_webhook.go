package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func VerifyZarinPalWebhookSignature(secret string, data map[string]string, signature string) bool {
	// Based on ZarinPal documentation: signature = HMAC-SHA256(amount + authority + status + secret)
	signatureData := data["amount"] + data["authority"] + data["status"] + secret

	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(signatureData))
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	return strings.EqualFold(expectedSignature, signature)
}
