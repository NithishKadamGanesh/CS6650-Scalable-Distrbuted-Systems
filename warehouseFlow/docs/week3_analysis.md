# Week 3 Analysis — Experiments 2 & 3

**Date completed:** Week 3
**Status:** ✅ Complete

---

## Experiment 2 — Resilience Under Warehouse Node Failure

### Setup
- 3 warehouse nodes (A, B, C), independent Redis each
- 3,000 orders/minute sustained load
- t=30s: failure injected (Warehouse B Redis stopped)
- t=150s: recovery (Warehouse B restored)
- 3 independent runs

### Results (avg of 3 runs)

| Metric | Result | Target | Status |
|---|---|---|---|
| Failover time | 3,233 ms | < 10,000 ms | ✅ |
| P99 spike at failure | 81.6 ms | — | Bounded |
| Orders permanently lost | **0** | 0 | ✅ |
| DLQ peak depth | 23.7 | — | All reprocessed |
| DLQ drained after recovery | Yes | Yes | ✅ |
| Full recovery time | 6,067 ms | < 60,000 ms | ✅ |
| Success rate floor | 62.1% | — | Brief dip only |

### Analysis

**Failover (t=30s):** Lazy fault detection via `Ping()` on every routing call detected B's failure in ~3 routing cycles. The failure window lasted 3.2 seconds.

**Degraded op:** 2-warehouse operation stable at 100% success. P99 rose slightly (33 → 36ms) as A and C absorbed extra load.

**DLQ capture:** 24 in-flight orders at moment of failure were correctly captured in SQS DLQ. After recovery, all 24 reprocessed. Zero permanently lost.

**Recovery:** Took ~6 seconds — longer than failover. The extra time was inventory rehydration: routing engine requires B to pass both `Ping()` and a successful inventory read before re-inclusion.

### Surprise
Recovery took 2x longer than failover — inventory rehydration is the bottleneck, not Redis startup. Worth the cost to prevent sending orders to a partially-loaded node.

---

## Experiment 3 — Write Contention Under Inventory Scarcity

### Setup
- Pre-seeded: 100 units of SKU-HOTITEM in Warehouse A
- Load: 1,000 concurrent orders all requesting SKU-HOTITEM
- Strategies: Optimistic (CAS) vs Pessimistic (SETNX)
- 5 runs per strategy

### Results (avg of 5 runs each)

| Metric | Optimistic | Pessimistic |
|---|---|---|
| Orders routed | 100 | 100 |
| Orders rejected | 875 | 900 |
| **Oversells** | **0** | **0** |
| Retry rate | 41.5% | 0% |
| Avg latency | 8.4 ms | 14.9 ms |
| P99 latency | 91.2 ms | 126.7 ms |
| Throughput | **187.4 orders/sec** | 80.4 orders/sec |

### Analysis — Headline Result

**Both strategies achieved zero oversells.**

This contradicted the hypothesis that optimistic would produce 5–15 oversells. The actual correctness guarantee is the Lua atomic decrement — the CAS script atomically checks-and-decrements, so even without a distributed lock, no two goroutines can claim the same unit.

The client-side locking strategy only affects throughput and latency profiles, not correctness, as long as the underlying operation is atomic.

### Practical Tradeoff
- **Optimistic:** 2.3x faster at cost of 41.5% retry rate
- **Pessimistic:** Zero retries at cost of serialization latency

Neither produces oversells under 1000:100 contention. Choose based on binding constraint: throughput or retry overhead.

---

## Week 3 Exit Criterion
✅ Both experiments complete. Surprising findings documented (inventory rehydration cost, optimistic correctness).
