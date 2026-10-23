# Chaos Test 03: Runaway Loop (Load Shedding)

**Objective:** Validate that JaguarBase protects the internal DuckDB engine from malicious or buggy AI Agents executing runaway infinite loops, employing $O(1)$ Load Shedding at the Go Edge plane.

## Scenario
* **The Threat:** An Autonomous Agent gets stuck in a loop, firing thousands of JSON-RPC vector search requests instantly, aiming to exhaust the server's thread pool and CPU.
* **The Defense:** An atomic lock-free `inFlightCounter` acts as a dynamic Load Shedding barrier on the HTTP multiplexer. Configured via `jaguardb.yaml` (e.g., limit: 100), it rejects excessive packets instantly with `HTTP 429 Too Many Requests`, before routing any computation to the C++ database layer.
* **Test Mock:** A massive burst of 2,000 JSON-RPC requests fired concurrently at the MCP port (8080) with 1,000 active workers.

## Benchmark Results (Intel i7-7500U Dual-Core)

| Metric | Result |
| :--- | :--- |
| **Concurrency Level** | 1,000 workers |
| **Total Requests Fired** | 2,000 |
| **Test Duration** | **574.42ms** |
| **Success (Processed by DuckDB)** | **311 requests** |
| **Shedded (Rejected by Go Router)** | **1,689 requests (429 Too Many Requests)** |
| **Status** | 🏆 **VICTORY** (Engine fully protected) |

## Architectural Insights

1. **Zero-Overhead Rejection:** The Go HTTP router rejected 1,689 requests in just over half a second using lock-free atomics (`atomic.Int32`). This proves the system enforces traffic shaping at the network edge with $O(1)$ complexity, ensuring malicious spikes do not degrade legitimate operations.
2. **Fail-Safe Config:** The shedding threshold is strictly decoupled from the binary, injected dynamically via the `jaguardb.yaml` manifest (via `Manifest.Security.MCPInFlightLimit`), maintaining the system's agnostic and cloud-native posture.