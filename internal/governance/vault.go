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

// Tokenize takes a column name and a raw sensitive string, stores it securely,
// and returns an agnostic, HMAC-protected deterministic token: vault_{column}_{hash}.
func (v *Vault) Tokenize(ctx context.Context, columnName string, raw string) (string, error) {
	if raw == "" {
		return "", nil // Edge case: do not mask empty strings
	}

	col := strings.ToLower(strings.TrimSpace(columnName))
	if col == "" {
		col = "pii"
	}

	// Generate deterministic keyed HMAC-SHA256 hash to prevent rainbow table attacks
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(raw))
	sum := mac.Sum(nil)
	token := fmt.Sprintf("vault_%s_%s", col, hex.EncodeToString(sum)[:16])

	// Store the real value in Redis
	key := vaultKeyPrefix + token
	err := v.client.Set(ctx, key, raw, v.ttl).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store token in vault: %w", err)
	}

	return token, nil
}
