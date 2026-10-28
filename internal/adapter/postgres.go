package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresAdapter manages the physical connection to the source database.
type PostgresAdapter struct {
	pool *pgxpool.Pool
}

// NewPostgresAdapter initializes a high-performance connection pool.
func NewPostgresAdapter(ctx context.Context, connString string) (*PostgresAdapter, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	// Optimizations for heavy traffic (ideal for fast data ingestion)
	config.MaxConns = 10
	config.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	// Test the connection (ping)
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres unreachable: %w", err)
	}

	return &PostgresAdapter{pool: pool}, nil
}

// Close gracefully terminates the connection pool during shutdown.
func (p *PostgresAdapter) Close() {
	if p.pool != nil {
		p.pool.Close()
	}
}

// FetchNew performs a dynamic CDC fetch for new records in the agnostic table.
// It returns a slice of generic maps, completely decoupling the Go code from the database schema.
func (p *PostgresAdapter) FetchNew(ctx context.Context, tableName string, columns []string, pkColumn string, lastID int, limit int) ([]map[string]any, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("no columns provided for dynamic fetch")
	}

	// Build the SELECT query dynamically
	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s > $1 ORDER BY %s ASC LIMIT $2",
		strings.Join(columns, ", "),
		tableName,
		pkColumn,
		pkColumn,
	)

	rows, err := p.pool.Query(ctx, query, lastID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch records from table '%s': %w", tableName, err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		// Allocate slice of empty interfaces and their pointers to scan unknown columns
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Map column names back to their scanned values
		rowMap := make(map[string]any, len(columns))
		for i, col := range columns {
			val := values[i]
			// Ensure bytes are converted to strings if returned as such by pgx
			if b, ok := val.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = val
			}
		}

		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return results, nil
}
