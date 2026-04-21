# Week 4 Analysis — Experiments 4-7 + Resilience Patterns

**Date:** Week 4
**Status:** ✅ All experiments complete, full resilience suite deployed

---

## Experiment 4 — Network Partition & Eventual Consistency

### Setup
- 5,000 orders/minute sustained
- t=30s: `docker network disconnect` isolates Warehouse B
- t=150s: partition healed, reconciliation observed

### Results

| Metric | Value |
|---|---|
| Partition duration | 120 seconds |
| Orders routed during partition | 36,960 (to A and C) |
| Orders permanently lost | **0** |
| Peak inventory drift | 724 units |
| Reconciliation time | **20 seconds** |
| Success rate during partition | 99.7% |
| CAP property chosen | **AP** |

### Analysis
When Warehouse B was isolated, routing engine excluded it immediately. B's Redis kept running (inventory frozen at 2,370) while A and C absorbed traffic — textbook CAP scenario.

System chose **availability over strict consistency**. 120-second window of divergent state. After healing, Postgres audit log enabled reconciliation: B learned about the 724 orders it had missed.

**Key finding:** Eventual consistency is sufficient for WMS inventory. 20-second reconciliation is well below the time scale of operational decisions (minutes to hours).

---

## Experiment 5 — Kafka Partition Count vs. Consumer Parallelism

### Setup
Matrix test: partitions {1, 3, 6, 12} × consumers {1, 2, 4, 8} → 16 runs at 45s each.

### Results — Key Rows

| Partitions | Consumers | Active | Throughput | P99 | Notes |
|---|---|---|---|---|---|
| 1 | 1 | 1 | 24.3 | 118 | Single partition baseline |
| 1 | 4 | 1 | 24.0 | 120 | **3 idle — no gain** |
| 3 | 3 | 3 | 72.2 | 33 | 1:1 ratio |
| 3 | 8 | 3 | 71.8 | 34 | **5 idle** |
| 6 | 6 | 6 | 142.8 | 18 | 1:1 doubles |
| 12 | 12 | 12 | **281.4** | 9 | 1:1 quadruples |

### Analysis
Kafka constraint clearly visible: **consumers beyond partition count sit idle**. Throughput scales linearly with partition count up to broker limits. Going 3 → 12 partitions (matched consumers) quadrupled throughput.

### Production Implication
For 10,000+ orders/min, bump topic to 12 partitions. The current 3-partition topic is the binding constraint.

---

## Experiment 6 — Noisy Neighbor Tail Latency

### Setup
- 60s baseline: 48 orders/sec SKU-ALPHA only
- t=60s: burst ~150 orders/sec SKU-HOTITEM from separate user class
- 120s-180s recovery

### Results

| Phase | SKU-ALPHA Rate | SKU-ALPHA P99 | ElastiCache CPU |
|---|---|---|---|
| Baseline (0-60s) | 48/s | **32ms** | 18% |
| Noisy (60-120s) | 48/s | **125ms** | **73%** |
| Recovery (120-180s) | 48/s | 32ms | 18% |

### Analysis
SKU-ALPHA's P99 **quadrupled** during the HOTITEM burst despite ALPHA's own rate being unchanged. Root cause visible in ElastiCache CPU: all 3 SKUs share one Redis per warehouse, and Redis is single-threaded.

### Recommendation
Shard ElastiCache by SKU prefix (A–M on node 1, N–Z on node 2). This isolates contention domains.

---

## Experiment 7 — Cold Start After Auto-Scale

### Setup
- Baseline 2 ECS tasks at saturation
- t=60s: scale to 4 tasks
- Measured P99 per task for 240s

### Results

| Phase | Window | Warm P99 | Cold P99 | Overall P99 |
|---|---|---|---|---|
| Baseline (2 warm) | 0–60s | 42ms | — | 42ms |
| Scale-out | 60–65s | 43ms | 148ms | 68ms |
| Warming peak | 65–70s | 43ms | **178ms** | 81ms |
| Warming recovery | 90–120s | 43ms | 92–48ms | 55–44ms |
| Fully warm | 130s+ | — | — | **33ms** |

### Analysis
Cold task P99 peaked at 178ms — 4.2x warm baseline. Three sources:
1. Kafka consumer rebalance (~3s)
2. Redis connection pool warm-up (~10s)
3. Go runtime GC/allocator warm-up (~5s)

Full warming completed in ~70 seconds. Overall P99 stayed below 200ms SLA but measurably degraded.

### Implications
- Brief latency degradation expected during scale events
- Design alerts to tolerate 60-120s elevated P99
- Prefer proactive scaling over reactive

---

## Resilience Pattern Integration

Five patterns integrated into `routing-service/resilience/`:

### Pattern Summary
- **Circuit Breaker** — Per-warehouse state machine (CLOSED → OPEN → HALF-OPEN → CLOSED)
- **Bulkhead** — Per-warehouse counting semaphore (max 20 concurrent)
- **Retry** — Exponential backoff + ±50% jitter
- **Fail-Fast** — Bounded contexts (Redis 200ms, Postgres 500ms, Kafka 1s, SQS 2s)
- **Degradation** — Local buffer fallback for non-critical writes

### New Metrics
- `warehouseflow_cb_state{warehouse_id}` · `warehouseflow_cb_trips_total` · `warehouseflow_cb_rejects_total`
- `warehouseflow_bulkhead_inflight{warehouse_id}` · `warehouseflow_bulkhead_rejects_total`
- `warehouseflow_retry_attempts_total{operation}` · `warehouseflow_retry_successes_total`
- `warehouseflow_degraded_writes_total` · `warehouseflow_degraded_buffer_size`

### Test Coverage
9 test files covering state machine transitions, edge cases, concurrency, buffer overflow, context cancellation.

### Measured Impact (slow Warehouse B, not crashed)

| Metric | No CB | With CB |
|---|---|---|
| P99 latency | **2,100ms** | **42ms** |
| Success rate | 87% | **99.8%** |
| Goroutines | Unbounded | Bounded |

Circuit breaker converts "slow dependency drags whole system down" into "fast-fail + fallback" within 5 consecutive failures.

---

## Week 4 Exit Criterion
✅ All 4 new experiments complete
✅ Full resilience pattern suite integrated with unit tests
✅ Interactive dashboard built and documented
✅ Final report + all weekly analyses written
