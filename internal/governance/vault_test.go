package governance_test

import (
	"strings"
	"testing"

	"github.com/andrewbraga1/jaguarbase/internal/governance"
)

func TestVault_GenerateToken_128BitEntropy(t *testing.T) {
	secret := []byte("high-entropy-master-secret-key-999")
	vault := governance.NewVaultWithSecret(secret)

	token1 := vault.GenerateToken("cpf", "123.456.789-00")
	token2 := vault.GenerateToken("cpf", "123.456.789-00")
	tokenDiff := vault.GenerateToken("cpf", "123.456.789-01")

	// 1. Must be deterministic
	if token1 != token2 {
		t.Fatalf("Expected deterministic tokens, got %s != %s", token1, token2)
	}

	// 2. Different inputs must produce different tokens
	if token1 == tokenDiff {
		t.Fatalf("Collision detected between different inputs!")
	}

	// 3. Must have prefix 'vault_cpf_'
	if !strings.HasPrefix(token1, "vault_cpf_") {
		t.Errorf("Expected prefix 'vault_cpf_', got %s", token1)
	}

	// 4. Must have 32 hex characters (128 bits of entropy) after the prefix
	hashPart := strings.TrimPrefix(token1, "vault_cpf_")
	if len(hashPart) != 32 {
		t.Errorf("Expected 32 hex chars (128 bits entropy), got %d chars: %s", len(hashPart), hashPart)
	}

	t.Logf("[PASS] 128-bit Token: %s", token1)
}

func TestVault_AES256GCM_EncryptionAtRest(t *testing.T) {
	secret := []byte("high-entropy-master-secret-key-999")
	vault := governance.NewVaultWithSecret(secret)

	originalPII := "sensitive-cpf-999.888.777-66"

	// 1. Encrypt value
	ciphertextHex, err := vault.EncryptValue(originalPII)
	if err != nil {
		t.Fatalf("Failed to encrypt PII: %v", err)
	}

	// 2. Ciphertext must NOT contain plaintext
	if strings.Contains(ciphertextHex, originalPII) {
		t.Fatalf("CRITICAL SECURITY LEAK: Ciphertext contains raw plaintext!")
	}

	// 3. Decrypt value
	decrypted, err := vault.DecryptValue(ciphertextHex)
	if err != nil {
		t.Fatalf("Failed to decrypt ciphertext: %v", err)
	}

	if decrypted != originalPII {
		t.Fatalf("Decrypted value mismatch: got %q, expected %q", decrypted, originalPII)
	}

	// 4. Tampered ciphertext must fail (authenticated encryption)
	tamperedHex := ciphertextHex[:len(ciphertextHex)-2] + "ff"
	_, err = vault.DecryptValue(tamperedHex)
	if err == nil {
		t.Fatalf("Expected tamper detection failure, got nil error")
	}

	t.Logf("[PASS] AES-256-GCM encryption and authenticated tamper detection validated.")
}

