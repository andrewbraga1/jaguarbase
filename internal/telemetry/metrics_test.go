package telemetry_test

import (
	"sync"
	"testing"

	"github.com/andrewbraga1/jaguarbase/internal/telemetry"
)

func TestMetrics_Concurrency(t *testing.T) {
	var wg sync.WaitGroup
	routines := 100
	incrementsPerRoutine := 1000

	// Reset counters for clean test state
	telemetry.NodeMetrics.QueriesExecuted = 0
	telemetry.NodeMetrics.SecurityBlocks = 0

	t.Run("Safely increments metrics under high concurrency", func(t *testing.T) {
		wg.Add(routines)

		for i := 0; i < routines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < incrementsPerRoutine; j++ {
					telemetry.IncQueries()
					if j%2 == 0 {
						telemetry.IncSecurityBlocks()
					}
				}
			}()
		}

		// Wait for all 100k operations to finish
		wg.Wait()

		snap := telemetry.Snapshot()

		expectedQueries := uint64(routines * incrementsPerRoutine)
		if snap["queries_total"].(uint64) != expectedQueries {
			t.Errorf("Expected %d queries, got %d", expectedQueries, snap["queries_total"])
		}

		expectedBlocks := uint64(routines * (incrementsPerRoutine / 2))
		if snap["security_blocks"].(uint64) != expectedBlocks {
			t.Errorf("Expected %d security blocks, got %d", expectedBlocks, snap["security_blocks"])
		}

		t.Logf("[PASS] High Concurrency metrics computed perfectly: %v", snap)
	})
}
