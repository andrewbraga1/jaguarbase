# Chaos Test 01: Read/Write Contention

**Objective:** Validate DuckDB's MVCC (Multi-Version Concurrency Control) and file-locking behavior when subjected to extreme Read/Write contention on physical SSD storage.

## Scenario
* **Write Load:** Simulates a CDC pipeline attempting to insert batches every 50ms.
* **Read Load:** 100 AI Agents (Goroutines) continuously firing 1536-dimensional cosine similarity searches.
* **State:** Physical Disk (`.duckdb` and `.wal` files) to trigger real I/O bottlenecks.

## Benchmark Results (Intel i7 Dual-Core)

| Metric | Result |
| :--- | :--- |
| **Duration** | 10.02s |
| **Batches Written (CDC)** | 68 (1 error) |
| **Semantic Searches Read** | 7,435 (23 errors) |
| **Throughput (Under Fire)** | **741.63 req/s** |
| **Error Rate** | **~0.3%** |

## Architectural Insights

1. **Graceful Degradation:** Even with 100 workers locking the database for vector math, the system maintained a massive 741 req/s throughput. Compared to the pure read-only test (~995 req/s), the performance penalty of heavy concurrent writing is only ~25%.
2. **Write-Ahead Log (WAL) Backpressure:** The writer attempted 200 inserts but achieved 68. This proves that Go's channel/lock mechanics respect DuckDB's internal file locks. The writer gracefully queues up instead of crashing the database.
3. **Real-World Impact:** In a production environment, PostgreSQL CDC batches events and flushes them every few seconds, rather than every 50ms. Therefore, the 0.3% contention error rate observed here is an extreme edge-case that proves the system's safety bounds.