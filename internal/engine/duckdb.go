package engine

import (
	"context"
	"database/sql"
	"fmt"

	// Register DuckDB driver
	_ "github.com/marcboeker/go-duckdb"
)

// Ticket represents the safe, processed data model within the execution engine.
type Ticket struct {
	ID          int    `json:"id"`
	CustomerCPF string `json:"customer_cpf,omitempty"`
	Message     string `json:"message"`
	IsTainted   bool   `json:"is_tainted"`
}

// DuckDBEngine manages the embedded C++ analytical database.
type DuckDBEngine struct {
	db *sql.DB
}

// NewDuckDBEngine initializes an embedded DuckDB instance.
// For in-memory, pass "?access_mode=read_write" or "" as dsn.
func NewDuckDBEngine(ctx context.Context, dsn string) (*DuckDBEngine, error) {
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping duckdb: %w", err)
	}

	engine := &DuckDBEngine{db: db}
	if err := engine.InitSchema(ctx); err != nil {
		return nil, err
	}

	return engine, nil
}

// Close gracefully shuts down the embedded database.
func (e *DuckDBEngine) Close() error {
	return e.db.Close()
}

// InitSchema creates the strictly typed columnar table.
func (e *DuckDBEngine) InitSchema(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS safe_tickets (
			id INTEGER PRIMARY KEY,
			customer_cpf VARCHAR,
			message VARCHAR,
			is_tainted BOOLEAN
		);
	`
	_, err := e.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	return nil
}

// InsertBatch performs an atomic, low-allocation bulk insert.
func (e *DuckDBEngine) InsertBatch(ctx context.Context, tickets []Ticket) error {
	if len(tickets) == 0 {
		return nil
	}

	// Fail-Closed: Use transactions for atomicity
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Safe to call even if committed

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO safe_tickets (id, customer_cpf, message, is_tainted) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, t := range tickets {
		_, err := stmt.ExecContext(ctx, t.ID, t.CustomerCPF, t.Message, t.IsTainted)
		if err != nil {
			return fmt.Errorf("failed to execute insert for ticket %d: %w", t.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// QueryAllClean returns only safe records, pre-allocating memory for Zero-Allocation on growing slices.
func (e *DuckDBEngine) QueryAllClean(ctx context.Context) ([]Ticket, error) {
	// Filter out tainted data at the columnar level (SIMD accelerated)
	query := `SELECT id, customer_cpf, message, is_tainted FROM safe_tickets WHERE is_tainted = false ORDER BY id ASC`
	
	rows, err := e.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query clean tickets: %w", err)
	}
	defer rows.Close()

	// Pre-allocate capacity (assuming minimum 10 rows to reduce GC pressure)
	cleanTickets := make([]Ticket, 0, 10)

	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.CustomerCPF, &t.Message, &t.IsTainted); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		cleanTickets = append(cleanTickets, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return cleanTickets, nil
}