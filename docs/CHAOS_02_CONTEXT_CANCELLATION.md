# Chaos Test 02: Phantom Agent (Context Cancellation)

**Objective:** Validate that JaguarBase prevents CPU resource leaks by correctly propagating Go's `context.Context` cancellation signals down to the embedded C++ DuckDB engine, and measure the exact computational resources saved.

## Scenario
* **The Threat:** "Phantom Agents" — AI clients that initiate heavy JSON-RPC vector search requests but drop the TCP connection (disconnect) before the database finishes computing the cosine similarities.
* **The Defense:** Go's `net/http` router automatically cancels the request context. The database driver must intercept this signal and halt execution immediately.
* **Test Mock:** 50,000 vectors (1536-dim) are populated. We establish a **Baseline** compute time, then fire a **Phantom Query** with a hard `1ms` context timeout to simulate a mid-flight client disconnect.

## Benchmark Results (Intel i7-7500U Dual-Core)

| Metric | Result |
| :--- | :--- |
| **Baseline Compute Time** | **312.67ms** |
| **Phantom Abort Time** | **3.66ms** |
| **CPU Time Saved** | **309.00ms per phantom request** |
| **Resources Preserved** | **98.83%** |
| **Status** | 🛡️ **SUCCESS** (`context deadline exceeded`) |

## Architectural Insights

1. **Observability & Efficiency:** By measuring the baseline, we proved that dropping an orphaned request in ~3.6ms prevents the CPU from doing over 300ms of useless vector math. Over thousands of agent interactions, this translates to massive infrastructure cost savings and prevents cascading failures.
2. **Deterministic Shutdown:** Traditional standalone databases often struggle with orphaned queries, requiring complex `pg_cancel_backend` commands and timeout scripts to kill runaway math. JaguarBase handles it elegantly and natively via Go's concurrency primitives, shielding the engine from degradation.