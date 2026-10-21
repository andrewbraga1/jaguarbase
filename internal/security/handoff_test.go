package security_test

import (
	"testing"

	"github.com/andrewbraga1/jaguarbase/internal/security"
)

func TestHandoffSecurity(t *testing.T) {
	secretKey := []byte("jgate-super-secret-2026")
	payload := []byte(`{"status":"SUCCESS","total_rows":2}`)

	t.Run("Generates and Verifies Valid Proof", func(t *testing.T) {
		proof := security.GenerateFrameProof(secretKey, payload)

		if !security.VerifyFrameProof(secretKey, payload, proof) {
			t.Errorf("Expected valid proof to be verified successfully.")
		}
	})

	t.Run("Rejects Tampered Payload (Fail-Closed)", func(t *testing.T) {
		proof := security.GenerateFrameProof(secretKey, payload)
		tamperedPayload := []byte(`{"status":"SUCCESS","total_rows":9999}`) // Hacker changed rows

		if security.VerifyFrameProof(secretKey, tamperedPayload, proof) {
			t.Errorf("Security Breach: Tampered payload bypassed HMAC verification!")
		}
		t.Logf("[PASS] Tampered payload correctly rejected.")
	})
}
