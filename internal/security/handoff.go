package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateFrameProof creates a deterministic HMAC-SHA256 signature for a given payload.
// It uses zero-allocation principles for the hash computation.
func GenerateFrameProof(secretKey, payload []byte) string {
	mac := hmac.New(sha256.New, secretKey)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyFrameProof checks if the signature matches the payload.
// It uses constant-time comparison to mitigate timing attacks.
func VerifyFrameProof(secretKey, payload []byte, signature string) bool {
	expectedMAC := GenerateFrameProof(secretKey, payload)

	expectedBytes, _ := hex.DecodeString(expectedMAC)
	actualBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false // Fail-closed on invalid hex
	}

	return hmac.Equal(expectedBytes, actualBytes)
}
