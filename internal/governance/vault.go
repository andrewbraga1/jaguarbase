package governance

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const vaultKeyPrefix = "jbase:vault:"

// Vault handles bidirectional PII tokenization using Redis as the secure state store.
type Vault struct {
	client *redis.Client
	ttl    time.Duration
	secret []byte
}

// NewVault initializes the Redis connection and verifies reachability.
func NewVault(ctx context.Context, redisURL string) (*Vault, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	client := redis.NewClient(opt)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	secret := os.Getenv("JAGUARBASE_HMAC_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("JAGUARBASE_HMAC_SECRET environment variable is strictly required for Vault tokenization")
	}

	return &Vault{
		client: client,
		ttl:    24 * time.Hour, // Standard TTL for ephemeral agent sessions
		secret: []byte(secret),
	}, nil
}

// Close gracefully terminates the Redis connection pool.
func (v *Vault) Close() error {
	return v.client.Close()
}

// NewVaultWithSecret creates an in-memory Vault representation with secret key (for testing or token generation).
func NewVaultWithSecret(secret []byte) *Vault {
	return &Vault{
		ttl:    24 * time.Hour,
		secret: secret,
	}
}

// EncryptValue encrypts plaintext using AES-256-GCM derived from the vault secret.
func (v *Vault) EncryptValue(raw string) (string, error) {
	key := sha256.Sum256(v.secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(raw), nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptValue decrypts an AES-256-GCM hex string using the vault secret.
func (v *Vault) DecryptValue(ciphertextHex string) (string, error) {
	ciphertext, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		return "", fmt.Errorf("invalid hex ciphertext: %w", err)
	}
	key := sha256.Sum256(v.secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext payload too short")
	}
	nonce, cipherBytes := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed (authentication tag mismatch): %w", err)
	}
	return string(plaintext), nil
}

// GenerateToken derives a deterministic token with 128 bits of entropy (32 hex characters).
func (v *Vault) GenerateToken(columnName string, raw string) string {
	if raw == "" {
		return ""
	}

	col := strings.ToLower(strings.TrimSpace(columnName))
	if col == "" {
		col = "pii"
	}

	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(raw))
	sum := mac.Sum(nil)
	return fmt.Sprintf("vault_%s_%s", col, hex.EncodeToString(sum)[:32])
}

// Tokenize takes a column name and a raw sensitive string, encrypts it with AES-256-GCM,
// stores it securely in Redis, and returns an HMAC-protected deterministic token: vault_{column}_{hash}.
func (v *Vault) Tokenize(ctx context.Context, columnName string, raw string) (string, error) {
	if raw == "" {
		return "", nil // Edge case: do not mask empty strings
	}

	token := v.GenerateToken(columnName, raw)

	// Encrypt the sensitive value at rest before persisting in Redis
	encryptedVal, err := v.EncryptValue(raw)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt PII value: %w", err)
	}

	// Store the encrypted value in Redis
	key := vaultKeyPrefix + token
	err = v.client.Set(ctx, key, encryptedVal, v.ttl).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store token in vault: %w", err)
	}

	return token, nil
}

// Detokenize retrieves and decrypts the original PII value from the vault.
func (v *Vault) Detokenize(ctx context.Context, token string) (string, error) {
	key := vaultKeyPrefix + token
	encryptedVal, err := v.client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("token not found in vault: %w", err)
	}

	return v.DecryptValue(encryptedVal)
}
