package adapter_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/andrewbraga1/jaguarbase/internal/adapter"
)

// TestPostgresAdapter_Integration verifies the real connection to PostgreSQL.
// It requires a running database and the TEST_DATABASE_URL environment variable.
func TestPostgresAdapter_Integration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set. Skipping PostgreSQL integration test.")
	}

	// Use a strict timeout to prevent tests from hanging on bad connections
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("Successfully connects to PostgreSQL", func(t *testing.T) {
		pgAdapter, err := adapter.NewPostgresAdapter(ctx, dbURL)
		if err != nil {
			t.Fatalf("Failed to initialize PostgresAdapter: %v", err)
		}
		defer pgAdapter.Close()

		t.Logf("[PASS] Connection pool established successfully.")
	})

	t.Run("Fetches new records dynamically", func(t *testing.T) {
		pgAdapter, err := adapter.NewPostgresAdapter(ctx, dbURL)
		if err != nil {
			t.Fatalf("Failed to initialize PostgresAdapter: %v", err)
		}
		defer pgAdapter.Close()

		// Agnostic parameters driven by the YAML manifestation logic
		tableName := "support_tickets"
		columns := []string{"id", "customer_cpf", "message"}
		pkColumn := "id"

		// Fetch rows starting from ID 0, with a limit of 10
		records, err := pgAdapter.FetchNew(ctx, tableName, columns, pkColumn, 0, 10)
		if err != nil {
			t.Fatalf("FetchNew failed: %v", err)
		}

		if len(records) == 0 {
			t.Logf("[WARN] No records found. Make sure the '%s' table is seeded.", tableName)
		} else {
			t.Logf("[PASS] Fetched %d records successfully.", len(records))

			// Validate dynamic map mapping instead of hardcoded struct
			for _, record := range records {
				idVal, hasID := record["id"]
				if !hasID || idVal == nil {
					t.Errorf("Expected record to contain an 'id' key")
				}

				msgVal, hasMsg := record["message"]
				if !hasMsg || msgVal == "" {
					t.Errorf("Expected record to contain a non-empty 'message' for ID %v", idVal)
				}
			}
		}
	})
}
