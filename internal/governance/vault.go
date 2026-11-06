package governance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

// Tokenize takes a column name and a raw sensitive string, stores it securely,
// and returns an agnostic, HMAC-protected deterministic token: vault_{column}_{hash}.
func (v *Vault) Tokenize(ctx context.Context, columnName string, raw string) (string, error) {
	if raw == "" {
		return "", nil // Edge case: do not mask empty strings
	}

	token := v.GenerateToken(columnName, raw)

	// Store the real value in Redis
	key := vaultKeyPrefix + token
	err := v.client.Set(ctx, key, raw, v.ttl).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store token in vault: %w", err)
	}

	return token, nil
}
