package ingestion

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/lib/pq" // Necessário para o sql.Open funcionar com postgres
)

// NewAdapter is a Factory that returns the correct DataSource implementation
// based on the connection string scheme (e.g., postgres://, mysql://).
func NewAdapter(dsn string, tableName string) (DataSource, error) {
	parsedURL, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN format: %w", err)
	}

	scheme := strings.ToLower(parsedURL.Scheme)

	switch scheme {
	case "postgres", "postgresql":
		// 1. Abre a conexão no dialeto correto
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres connection: %w", err)
		}

		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping postgres: %w", err)
		}

		// 2. Define o canal do LISTEN/NOTIFY dinamicamente
		channel := "new_" + tableName
		if tableName == "support_tickets" {
			channel = "new_support_ticket" // Mantém compatibilidade com o nosso gatilho atual
		}

		// 3. Retorna o Adapter (DataSource) e o erro nil
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
