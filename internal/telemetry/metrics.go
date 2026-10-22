package telemetry

import (
	"sync/atomic"
	"time"
)

// Metrics holds the atomic counters for the entire database node.
// Using uint64 and sync/atomic guarantees thread-safety at 100k+ ops/sec without Mutex locks.
type Metrics struct {
	QueriesExecuted uint64
	RowsIngested    uint64
	SecurityBlocks  uint64 // Incremented when PII Vault fails or Prompt Injection is detected
	ActiveConns     int64  // Can go up and down
	UptimeStart     time.Time
}

// Global instance for the node
var NodeMetrics = &Metrics{
	UptimeStart: time.Now(),
}

// IncQueries safely increments the query counter.
func IncQueries() {
	atomic.AddUint64(&NodeMetrics.QueriesExecuted, 1)
}

// AddIngestedRows safely adds a batch size to the ingested counter.
func AddIngestedRows(count uint64) {
	atomic.AddUint64(&NodeMetrics.RowsIngested, count)
}

// IncSecurityBlocks safely increments the intercepted threats counter.
func IncSecurityBlocks() {
	atomic.AddUint64(&NodeMetrics.SecurityBlocks, 1)
}

// AddConn adjusts the active connections counter (can be negative on disconnect).
func AddConn(delta int64) {
	atomic.AddInt64(&NodeMetrics.ActiveConns, delta)
}

// Snapshot returns a point-in-time copy of the metrics for JSON/Prometheus rendering.
func Snapshot() map[string]any {
	return map[string]any{
		"uptime_seconds":  time.Since(NodeMetrics.UptimeStart).Seconds(),
		"queries_total":   atomic.LoadUint64(&NodeMetrics.QueriesExecuted),
		"rows_ingested":   atomic.LoadUint64(&NodeMetrics.RowsIngested),
		"security_blocks": atomic.LoadUint64(&NodeMetrics.SecurityBlocks),
		"active_conns":    atomic.LoadInt64(&NodeMetrics.ActiveConns),
	}
}
