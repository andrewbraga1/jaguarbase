package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const vaultKeyPrefix = "jbase:vault:"

// Vault handles bidirectional PII tokenization using Redis as the secure state store.
type Vault struct {
	client *redis.Client
	ttl    time.Duration
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

	return &Vault{
		client: client,
		ttl:    24 * time.Hour, // Standard TTL for ephemeral agent sessions
	}, nil
}

// Close gracefully terminates the Redis connection pool.
func (v *Vault) Close() error {
	return v.client.Close()
}

// Tokenize takes a raw PII string, stores it securely, and returns a deterministic token.
func (v *Vault) Tokenize(ctx context.Context, raw string) (string, error) {
	if raw == "" {
		return "", nil // Edge case: do not mask empty strings
	}

	// Generate deterministic hash for the token
	hash := sha256.Sum256([]byte(raw))
	token := "vault_cpf_" + hex.EncodeToString(hash[:])[:8]

	// Store the real value in Redis
	key := vaultKeyPrefix + token
	err := v.client.Set(ctx, key, raw, v.ttl).Err()
	if err != nil {
		return "", fmt.Errorf("failed to store token in vault: %w", err)
	}

	return token, nil
}
