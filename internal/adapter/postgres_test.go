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

	t.Run("Fetches new tickets correctly", func(t *testing.T) {
		pgAdapter, err := adapter.NewPostgresAdapter(ctx, dbURL)
		if err != nil {
			t.Fatalf("Failed to initialize PostgresAdapter: %v", err)
		}
		defer pgAdapter.Close()

		// Fetch rows starting from ID 0, with a limit of 10
		tickets, err := pgAdapter.FetchNew(ctx, 0, 10)
		if err != nil {
			t.Fatalf("FetchNew failed: %v", err)
		}

		if len(tickets) == 0 {
			t.Logf("[WARN] No tickets found. Make sure the 'support_tickets' table is seeded.")
		} else {
			t.Logf("[PASS] Fetched %d tickets successfully.", len(tickets))

			// Validate struct mapping
			for _, ticket := range tickets {
				if ticket.ID <= 0 {
					t.Errorf("Expected ticket ID to be strictly positive, got %d", ticket.ID)
				}
				if ticket.Message == "" {
					t.Errorf("Expected ticket message to not be empty for ID %d", ticket.ID)
				}
			}
		}
	})
}
