package governance_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/adapter"
	"github.com/andrewbraga1/jaguarbase/internal/governance"
)

// TestGovernancePipeline_Integration connects Postgres to Redis, validating the Zero-Trust flow.
func TestGovernancePipeline_Integration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	redisURL := os.Getenv("TEST_REDIS_URL")

	if dbURL == "" || redisURL == "" {
		t.Skip("TEST_DATABASE_URL or TEST_REDIS_URL not set. Skipping integration test.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if os.Getenv("JAGUARBASE_HMAC_SECRET") == "" {
		t.Setenv("JAGUARBASE_HMAC_SECRET", "test-secret-key-123")
	}

	// 1. Initialize Adapters & Governance Modules
	pg, err := adapter.NewPostgresAdapter(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to init Postgres: %v", err)
	}
	defer pg.Close()

	vault, err := governance.NewVault(ctx, redisURL)
	if err != nil {
		t.Fatalf("Failed to init Redis Vault: %v", err)
	}
	defer vault.Close()

	scanner := governance.NewScanner()

	// 2. Fetch real data from Postgres dynamically
	tableName := "support_tickets"
	columns := []string{"id", "customer_cpf", "message"}
	pkColumn := "id"

	records, err := pg.FetchNew(ctx, tableName, columns, pkColumn, 0, 10)
	if err != nil {
		t.Fatalf("Failed to fetch records: %v", err)
	}

	if len(records) == 0 {
		t.Skip("No records found in database to test governance pipeline.")
	}

	// 3. Process each record through the Governance Pipeline
	t.Run("Apply Zero-Trust Pipeline", func(t *testing.T) {
		for _, record := range records {
			idVal := record["id"]

			// Apply Taint Check dynamically
			if msgVal, ok := record["message"].(string); ok {
				isTainted := scanner.CheckTaint(msgVal)
				record["_is_tainted"] = isTainted
			}

			// Apply Vault Tokenization dynamically
			if cpfVal, ok := record["customer_cpf"].(string); ok && cpfVal != "" {
				token, err := vault.Tokenize(ctx, "cpf", cpfVal)
				if err != nil {
					t.Fatalf("Vault failed for record %v: %v", idVal, err)
				}

				if !strings.HasPrefix(token, "vault_cpf_") {
					t.Errorf("Expected masked token, got %s", token)
				}
				record["customer_cpf"] = token // Mutate map with safe token
			}

			// Validate and log output
			t.Logf("[PASS] Processed Record %v | Tainted: %v | Safe CPF: %v", idVal, record["_is_tainted"], record["customer_cpf"])
		}
	})
}
