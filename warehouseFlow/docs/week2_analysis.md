# Week 2 Analysis — Experiment 1: Throughput vs. ECS Routing Task Count

**Date completed:** Week 2
**Status:** ✅ Complete

---

## Setup

- Fixed order arrival rate: **5,000 orders/min** (84 Locust users)
- ECS routing tasks tested: 1, 2, 4, 8
- 30 seconds per configuration, 2 runs each
- Saturation sweep: 4 tasks fixed, load varied 1,000 → 8,000 orders/min

---

## Results

| ECS Tasks | Actual RPS | P95 (ms) | P99 (ms) | CPU/Task | Kafka Lag | Bottleneck |
|---|---|---|---|---|---|---|
| 1 | 71.2 | 52.1 | 118.3 | 91.4% | 1,840 | CPU saturated |
| 2 | 83.1 | 28.3 | 61.4 | 48.2% | 220 | Near capacity |
| **4** | **84.0** | **14.7** | **32.8** | 24.1% | 0 | **Meets target** ✅ |
| 8 | 84.0 | 16.2 | 36.1 | 12.8% | 0 | ElastiCache pressure |

### Saturation Sweep (4 tasks)

| Target | Actual | P99 | Kafka Lag | Status |
|---|---|---|---|---|
| 1k | 1,002 | 30ms | 0 | ✅ Comfortable |
| 5k | 5,040 | 34ms | 0 | ✅ At capacity |
| 6k | 5,844 | 71ms | 412 | ⚠️ Lag building |
| 7k | 6,492 | 135ms | 2,108 | ⚠️ Saturating |
| 8k | 6,876 | 224ms | 5,891 | ❌ P99 > 200ms SLA |

---

## Analysis

### Hypothesis vs. Reality

**Hypothesis:** Near-linear improvement 1→4 tasks, diminishing returns at 8, ElastiCache as next bottleneck.

**Confirmed:**
- 1 task clearly CPU-bound (91.4% CPU, P99 = 118ms)
- 2 tasks = 48% P99 improvement
- 4 tasks hits 5,000 orders/min with P99 = 32.8ms ✅
- Saturation at ~5,500 orders/min

**Surprise — 8 tasks:**
- Throughput did NOT improve beyond 4 tasks
- P99 actually *increased* slightly (32.8 → 36.1ms)
- ElastiCache command latency rose from 1.1ms → 2.4ms
- **Conclusion:** The bottleneck at 8 tasks shifted to ElastiCache, confirming the hypothesis, but manifested as latency increase rather than throughput cap

### Key Insight
Saturation sweep is the most informative data. At 4 tasks, P99 stays under 200ms up to ~5,500 orders/min. This defines the operational ceiling for 4 ECS tasks.

---

## Week 2 Exit Criterion
✅ Experiment 1 fully complete. Bottleneck identified. Results match hypothesis with documented surprise at 8-task behavior.
