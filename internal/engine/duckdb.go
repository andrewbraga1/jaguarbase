package engine

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	_ "github.com/marcboeker/go-duckdb"
)

// DuckDBEngine manages the embedded C++ analytical database dynamically.
type DuckDBEngine struct {
	db *sql.DB
}

// NewDuckDBEngine initializes an embedded DuckDB instance.
func NewDuckDBEngine(ctx context.Context, dsn string) (*DuckDBEngine, error) {
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping duckdb: %w", err)
	}

	return &DuckDBEngine{db: db}, nil
}

// Close gracefully shuts down the database.
func (e *DuckDBEngine) Close() error {
	return e.db.Close()
}

// InitSchema dynamically creates a columnar table based on the YAML schema.
func (e *DuckDBEngine) InitSchema(ctx context.Context, tableName string, schema []config.ColumnDef) error {
	var colsBuilder strings.Builder
	for _, col := range schema {
		// Map YAML types to DuckDB SQL types using tagged switch (QF1003 fix)
		sqlType := "VARCHAR" // Default fallback
		switch col.Type {
		case "integer":
			sqlType = "INTEGER"
		case "boolean":
			sqlType = "BOOLEAN"
		}

		colsBuilder.WriteString(col.Column)
		colsBuilder.WriteString(" ")
		colsBuilder.WriteString(sqlType)
		colsBuilder.WriteString(", ")
	}

	// Always append the system security flag
	colsBuilder.WriteString("_is_tainted BOOLEAN")

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", tableName, colsBuilder.String())

	if _, err := e.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to initialize dynamic schema: %w", err)
	}
	return nil
}

// InsertBatch dynamically prepares and executes a bulk insert for maps.
func (e *DuckDBEngine) InsertBatch(ctx context.Context, tableName string, schema []config.ColumnDef, rows []map[string]any) error {
	if len(rows) == 0 {
		return nil
	}

	// Build dynamic SQL components without inefficient concatenation
	var colsBuilder, placeholdersBuilder strings.Builder
	for _, col := range schema {
		colsBuilder.WriteString(col.Column)
		colsBuilder.WriteString(", ")
		placeholdersBuilder.WriteString("?, ")
	}
	colsBuilder.WriteString("_is_tainted")
	placeholdersBuilder.WriteString("?")

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", tableName, colsBuilder.String(), placeholdersBuilder.String())

	// Fail-Closed: Atomicity
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, rowMap := range rows {
		// Extract values in the exact order of the schema
		args := make([]any, 0, len(schema)+1)
		for _, col := range schema {
			args = append(args, rowMap[col.Column])
		}
		// Append system flag
		isTainted := false
		if val, ok := rowMap["_is_tainted"].(bool); ok {
			isTainted = val
		}
		args = append(args, isTainted)

		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("failed to execute dynamic insert: %w", err)
		}
	}

	return tx.Commit()
}

// QueryAllClean dynamically selects safe records and maps them back to generic structures.
func (e *DuckDBEngine) QueryAllClean(ctx context.Context, tableName string, schema []config.ColumnDef) ([]map[string]any, error) {
	var colsBuilder strings.Builder
	for _, col := range schema {
		colsBuilder.WriteString(col.Column)
		colsBuilder.WriteString(", ")
	}
	colsBuilder.WriteString("_is_tainted")

	query := fmt.Sprintf("SELECT %s FROM %s WHERE _is_tainted = false", colsBuilder.String(), tableName)

	dbRows, err := e.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query clean records: %w", err)
	}
	defer dbRows.Close()

	columns, err := dbRows.Columns()
	if err != nil {
		return nil, err
	}

	cleanData := make([]map[string]any, 0, 10)

	for dbRows.Next() {
		// Dynamic scanning logic for unknown column structures
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := dbRows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan dynamic row: %w", err)
		}

		rowMap := make(map[string]any, len(columns))
		for i, colName := range columns {
			val := values[i]
			// DuckDB might return []byte for strings in some generic setups, ensure safe casting
			if b, ok := val.([]byte); ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}
		cleanData = append(cleanData, rowMap)
	}

	// Prevent silent failures (sqlrowserr fix)
	if err := dbRows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return cleanData, nil
}
