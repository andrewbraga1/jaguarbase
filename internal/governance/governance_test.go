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

	// 2. Fetch real data from Postgres
	tickets, err := pg.FetchNew(ctx, 0, 10)
	if err != nil {
		t.Fatalf("Failed to fetch tickets: %v", err)
	}

	if len(tickets) == 0 {
		t.Skip("No tickets found in database to test governance pipeline.")
	}

	// 3. Process each ticket through the Governance Pipeline
	t.Run("Apply Zero-Trust Pipeline", func(t *testing.T) {
		for _, ticket := range tickets {

			// Apply Taint Check
			ticket.IsTainted = scanner.CheckTaint(ticket.Message)

			// Apply Vault Tokenization
			if ticket.CustomerCPF != "" {
				token, err := vault.Tokenize(ctx, ticket.CustomerCPF)
				if err != nil {
					t.Fatalf("Vault failed for ticket %d: %v", ticket.ID, err)
				}

				if !strings.HasPrefix(token, "vault_cpf_") {
					t.Errorf("Expected masked token, got %s", token)
				}
				ticket.CustomerCPF = token // Mutate struct with safe token
			}

			// Validate and log output
			t.Logf("[PASS] Processed Ticket %d | Tainted: %v | Safe CPF: %s", ticket.ID, ticket.IsTainted, ticket.CustomerCPF)
		}
	})
}
