package adapter

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Ticket reflete a nossa entidade de domínio central.
type Ticket struct {
	ID          int    `json:"id"`
	CustomerCPF string `json:"customer_cpf,omitempty"`
	Message     string `json:"message"`
	IsTainted   bool   `json:"is_tainted"`
}

// PostgresAdapter gerencia a conexão física com o banco legado.
type PostgresAdapter struct {
	pool *pgxpool.Pool
}

// NewPostgresAdapter inicializa o pool de conexões de alta performance.
func NewPostgresAdapter(ctx context.Context, connString string) (*PostgresAdapter, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("erro ao parsear DSN: %w", err)
	}

	// Otimizações para tráfego intenso (ideal para ingestão rápida)
	config.MaxConns = 10
	config.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar no postgres: %w", err)
	}

	// Testa a conexão (ping)
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres inalcançável: %w", err)
	}

	return &PostgresAdapter{pool: pool}, nil
}

// Close encerra as conexões graciosamente no shutdown.
func (p *PostgresAdapter) Close() {
	if p.pool != nil {
		p.pool.Close()
	}
}

// FetchNew simula a etapa do CDC: busca registros novos na tabela do Secretário IA.
func (p *PostgresAdapter) FetchNew(ctx context.Context, lastID int, limit int) ([]Ticket, error) {
	// Substitua 'support_tickets' pelo nome real da tabela no seu banco
	query := `
		SELECT id, customer_cpf, message 
		FROM support_tickets 
		WHERE id > $1 
		ORDER BY id ASC 
		LIMIT $2
	`

	rows, err := p.pool.Query(ctx, query, lastID, limit)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar tickets: %w", err)
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		// Note que IsTainted não vem do Postgres, pois é gerado no JaguarBase!
		if err := rows.Scan(&t.ID, &t.CustomerCPF, &t.Message); err != nil {
			return nil, fmt.Errorf("erro ao ler linha: %w", err)
		}
		tickets = append(tickets, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante iteração: %w", err)
	}

	return tickets, nil
}
