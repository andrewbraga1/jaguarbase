package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/andrewbraga1/jaguarbase/internal/config"
	_ "github.com/marcboeker/go-duckdb"
)

type DuckDBEngine struct {
	db *sql.DB
}

func NewDuckDBEngine(ctx context.Context, dsn string) (*DuckDBEngine, error) {
	db, err := sql.Open("duckdb", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping duckdb: %w", err)
	}

	// Bootstrap Vector Search Extension (VSS)
	_, _ = db.ExecContext(ctx, "INSTALL vss; LOAD vss;")

	return &DuckDBEngine{db: db}, nil
}

func (e *DuckDBEngine) Close() error {
	return e.db.Close()
}

func (e *DuckDBEngine) InitSchema(ctx context.Context, tableName string, schema []config.ColumnDef) error {
	var colsBuilder strings.Builder
	for _, col := range schema {
		sqlType := "VARCHAR"
		switch col.Type {
		case "integer":
			sqlType = "INTEGER"
		case "boolean":
			sqlType = "BOOLEAN"
		case "vector":
			if col.Dimensions <= 0 {
				return fmt.Errorf("vector column '%s' requires positive dimensions", col.Column)
			}
			sqlType = fmt.Sprintf("FLOAT[%d]", col.Dimensions)
		}

		colsBuilder.WriteString(col.Column)
		colsBuilder.WriteString(" ")
		colsBuilder.WriteString(sqlType)
		colsBuilder.WriteString(", ")
	}

	colsBuilder.WriteString("_is_tainted BOOLEAN")

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", tableName, colsBuilder.String())

	if _, err := e.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("failed to initialize dynamic schema: %w", err)
	}
	return nil
}

func (e *DuckDBEngine) InsertBatch(ctx context.Context, tableName string, schema []config.ColumnDef, rows []map[string]any) error {
	if len(rows) == 0 {
		return nil
	}

	// 1. Build columns and placeholders 100% dynamically based ONLY on the provided schema
	var cols []string
	var placeholders []string
	for _, col := range schema {
		cols = append(cols, col.Column)
		placeholders = append(placeholders, "?")
	}

	// strings.Join automatically handles trailing commas
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		tableName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

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
		// The argument array size now matches the schema length exactly
		args := make([]any, 0, len(schema))
		for _, col := range schema {
			val := rowMap[col.Column]

			switch v := val.(type) {
			case []float32, []float64:
				// 1. Automatically marshal known numeric slices
				if b, err := json.Marshal(v); err == nil {
					val = string(b)
				}
			default:
				// If the schema says it's a vector or FLOAT array, force serialization.
				colTypeUpper := strings.ToUpper(col.Type)
				if strings.Contains(colTypeUpper, "VECTOR") || strings.HasPrefix(colTypeUpper, "FLOAT[") {
					if b, err := json.Marshal(val); err == nil {
						val = string(b)
					}
				}
			}
			args = append(args, val)
		}

		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("failed to execute dynamic insert: %w", err)
		}
	}

	return tx.Commit()
}

func (e *DuckDBEngine) QueryVector(ctx context.Context, tableName string, schema []config.ColumnDef, vectorCol string, queryVector []float32, limit int) ([]map[string]any, error) {
	var colsBuilder strings.Builder
	for _, col := range schema {
		colsBuilder.WriteString(col.Column)
		colsBuilder.WriteString(", ")
	}
	colsBuilder.WriteString("_is_tainted")

	// Convert queryVector to string for safe cross-boundary CGO parameter passing
	vectorBytes, _ := json.Marshal(queryVector)

	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE _is_tainted = false ORDER BY array_cosine_distance(%s, ?::FLOAT[%d]) ASC LIMIT %d",
		colsBuilder.String(), tableName, vectorCol, len(queryVector), limit,
	)

	dbRows, err := e.db.QueryContext(ctx, query, string(vectorBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to execute vector query: %w", err)
	}
	defer dbRows.Close()

	return e.scanDynamicRows(dbRows)
}

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

	return e.scanDynamicRows(dbRows)
}

func (e *DuckDBEngine) scanDynamicRows(dbRows *sql.Rows) ([]map[string]any, error) {
	columns, err := dbRows.Columns()
	if err != nil {
		return nil, err
	}

	cleanData := make([]map[string]any, 0, 10)

	for dbRows.Next() {
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
			if b, ok := val.([]byte); ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}
		cleanData = append(cleanData, rowMap)
	}

	if err := dbRows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return cleanData, nil
}

// AutoMigrate ensures that all tables mapped in the Manifest exist in DuckDB before receiving data.
func (e *DuckDBEngine) AutoMigrate(ctx context.Context, manifest *config.Manifest) error {
	// Iterate over all data sources declared in jaguardb.example.yaml
	for _, src := range manifest.Sources {

		// Dynamically build the query based on the table name from the manifest
		// Note: In the future, "FLOAT[3]" can also come from a src.Dimension variable!
		query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id VARCHAR,
			content VARCHAR,
			metadata JSON,
			embedding FLOAT[3],
			_is_tainted BOOLEAN
		);`, src.Table)

		// Replace 'e.db' with your actual sql.DB connection variable inside DuckDBEngine
		if _, err := e.db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed to create or verify table %s: %w", src.Table, err)
		}

		log.Printf("🛠️ AutoMigrate: Vector table '%s' successfully synchronized in DuckDB.", src.Table)
	}

	return nil
}
