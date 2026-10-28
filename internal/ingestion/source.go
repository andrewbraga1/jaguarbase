package ingestion

import "context"

// RawRecord represents an agnostic data row fetched from any transactional source.
// Used primarily for real-time events captured via Watch.
type RawRecord struct {
	ID       string
	Content  string
	Metadata string // JSON payload for dynamic columns (CPF, status, dates)
	Table    string // The source table name for dynamic routing
}

// DataSource defines the contract for any external database adapter.
// It guarantees support for both historical backfilling and real-time streaming.
type DataSource interface {
	// FetchBatch retrieves a chunk of historical records for initial sync.
	// It relies on dynamic mapping guided by the YAML manifest.
	FetchBatch(ctx context.Context, tableName string, columns []string, pkColumn string, lastID int, limit int) ([]map[string]any, error)

	// Watch starts the real-time event listener (e.g., LISTEN/NOTIFY or CDC).
	Watch(ctx context.Context, events chan<- RawRecord) error

	// Close gracefully terminates the adapter connection.
	Close() error
}
