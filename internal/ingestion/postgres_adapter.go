package ingestion

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/lib/pq"
)

// PostgresAdapter connects to PostgreSQL and listens for real-time events.
type PostgresAdapter struct {
	db        *sql.DB
	connStr   string
	channel   string
	tableName string
	listener  *pq.Listener
}

// NewPostgresAdapter initializes a new adapter.
func NewPostgresAdapter(db *sql.DB, connStr, channel, tableName string) *PostgresAdapter {
	return &PostgresAdapter{
		db:        db,
		connStr:   connStr,
		channel:   channel,
		tableName: tableName,
	}
}

// Watch listens to the Postgres NOTIFY channel and fetches full records dynamically.
func (p *PostgresAdapter) Watch(ctx context.Context, events chan<- RawRecord) error {
	reportProblem := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			log.Printf("⚠️ [Postgres Adapter] Listener error: %v", err)
		}
	}

	p.listener = pq.NewListener(p.connStr, 10*time.Second, time.Minute, reportProblem)
	if err := p.listener.Listen(p.channel); err != nil {
		return fmt.Errorf("failed to listen on channel %s: %w", p.channel, err)
	}

	log.Printf("🎧 Postgres Adapter: Listening for '%s' on table '%s' via LISTEN/NOTIFY...", p.channel, p.tableName)

	for {
		select {
		case <-ctx.Done():
			return nil
		case notification := <-p.listener.Notify:
			if notification == nil {
				continue
			}

			// notification.Extra contains the Record ID emitted by the trigger
			recordID := notification.Extra
			record, err := p.fetchRecordDynamic(ctx, recordID)
			if err != nil {
				log.Printf("❌ [Postgres Adapter] Failed to fetch record %s: %v", recordID, err)
				continue
			}

			events <- *record
		}
	}
}

// fetchRecordDynamic queries the record and routes columns dynamically.
func (p *PostgresAdapter) fetchRecordDynamic(ctx context.Context, recordID string) (*RawRecord, error) {
	// #nosec G201 -- Identifier structure validated by manifest parser regex allowlist
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1", p.tableName)
	rows, err := p.db.QueryContext(ctx, query, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("record %s not found", recordID)
	}

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Prepare dynamic memory pointers to receive arbitrary column data
	values := make([]interface{}, len(cols))
	valuePtrs := make([]interface{}, len(cols))
	for i := range cols {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	var id, content string
	metaMap := make(map[string]interface{})

	// Route core fields and encapsulate the rest into the metadata map
	for i, colName := range cols {
		val := values[i]
		if val == nil {
			continue // Skip nulls
		}

		// Postgres driver often returns strings as []byte; convert them safely
		var finalVal interface{}
		if b, ok := val.([]byte); ok {
			finalVal = string(b)
		} else {
			finalVal = val
		}

		switch colName {
		case "id":
			id = fmt.Sprintf("%v", finalVal)
		case "message", "content":
			content = fmt.Sprintf("%v", finalVal)
		default:
			metaMap[colName] = finalVal
		}
	}

	metaBytes, err := json.Marshal(metaMap)
	if err != nil {
		return nil, fmt.Errorf("failed to generate metadata JSON: %w", err)
	}

	return &RawRecord{
		ID:       id,
		Content:  content,
		Metadata: string(metaBytes),
		Table:    p.tableName,
	}, nil
}

// Close gracefully shuts down the listener.
func (p *PostgresAdapter) Close() error {
	if p.listener != nil {
		return p.listener.Close()
	}
	return nil
}

// FetchBatch performs a dynamic fetch for historical records (Backfill).
// It queries the database using agnostic mapping and returns a slice of generic maps,
// completely decoupling the Ingestion pipeline from the physical schema.
func (p *PostgresAdapter) FetchBatch(ctx context.Context, tableName string, columns []string, pkColumn string, lastID int, limit int) ([]map[string]any, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("no columns provided for dynamic fetch")
	}

	// Build the SELECT query dynamically for historical syncing
	// #nosec G201 -- Identifier structure validated by manifest parser regex allowlist
	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s > $1 ORDER BY %s ASC LIMIT $2",
		strings.Join(columns, ", "),
		tableName,
		pkColumn,
		pkColumn,
	)

	// Note: Assuming 'p.db' is the *sql.DB instance in your ingestion PostgresAdapter
	rows, err := p.db.QueryContext(ctx, query, lastID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch historical records from table '%s': %w", tableName, err)
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
			// Ensure bytes are converted to strings if returned as such by the driver
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
