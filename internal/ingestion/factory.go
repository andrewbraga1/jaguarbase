package ingestion

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	// pq driver is required for sql.Open to recognize the "postgres" dialect.
	_ "github.com/lib/pq"
)

// NewAdapter is a Factory that returns the correct DataSource implementation
// dynamically based on the connection string scheme (e.g., postgres://).
func NewAdapter(dsn string, tableName string) (DataSource, error) {
	parsedURL, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN format: %w", err)
	}

	scheme := strings.ToLower(parsedURL.Scheme)

	switch scheme {
	case "postgres", "postgresql":
		// 1. Establish the connection pool using the standard Go database/sql driver
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres connection: %w", err)
		}

		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping postgres: %w", err)
		}

		// 2. Define the CDC listener channel dynamically based strictly on the table name.
		// The DBA must configure their PostgreSQL trigger to notify on this specific channel.
		channel := fmt.Sprintf("new_%s", tableName)

		// 3. Return the fully initialized adapter that complies with the DataSource interface
		return NewPostgresAdapter(db, dsn, channel, tableName), nil

	case "mysql":
		// TODO: return NewMySQLAdapter(dsn) when implemented
		return nil, fmt.Errorf("mysql adapter is planned but not implemented yet")

	case "mongodb":
		// TODO: return NewMongoAdapter(dsn) when implemented
		return nil, fmt.Errorf("mongodb adapter is planned but not implemented yet")

	default:
		return nil, fmt.Errorf("unsupported database scheme: %s", scheme)
	}
}
