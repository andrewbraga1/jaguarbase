package ingestion

import "context"

// RawRecord represents an agnostic data row fetched from any transactional source.
type RawRecord struct {
	ID       string
	Content  string
	Metadata string // JSON payload for dynamic columns (CPF, status, dates)
	Table    string // The source table name for dynamic routing
}

// DataSource defines the contract for any external database adapter.
type DataSource interface {
	Watch(ctx context.Context, events chan<- RawRecord) error
	Close() error
}
