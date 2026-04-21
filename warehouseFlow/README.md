# WarehouseFlow
### Scalable Real-Time Order Routing Engine · CS 6650 Distributed Systems

A production-realistic WMS order routing backend that scales horizontally, survives partial failures, and handles write contention under extreme scarcity — validated through 7 controlled experiments.

> **Domain motivation:** Based on direct professional experience with production WMS at Manhattan Associates. Every experiment targets a real operational failure mode encountered in industrial warehouse systems.

---

## Results At A Glance

| # | Experiment | Core Finding |
|---|---|---|
| 1 | Horizontal Scaling | **4 ECS tasks = 5,000 orders/min at P99=33ms** |
| 2 | Node Failure Resilience | **Zero orders lost · 3.2s failover · 6.1s recovery** |
| 3 | Inventory Contention | **Zero oversells — both strategies** (Lua atomic decrement is the real guarantee) |
| 4 | Network Partition | **AP chosen · 0 orders lost · 20s reconciliation** |
| 5 | Kafka Partition:Consumer Ratio | **Consumers > partitions = idle · 1:1 is optimal** |
| 6 | Noisy Neighbor | **Shared ElastiCache causes 4x P99 spike for unrelated SKUs** |
| 7 | Cold Start | **New ECS tasks peak at 178ms P99 · full warm-up in 70s** |

### Resilience Patterns (5)
Circuit Breaker · Bulkhead · Retry+Jitter · Fail-Fast · Graceful Degradation — all with unit tests + Prometheus metrics.

### Interactive Dashboard
`dashboard/index.html` · no backend required · live warehouse map · 7-experiment autopilot.

---

## Architecture

```
         Load Generator (Locust)
                ↓
    ┌──────────────────────────────┐
    │   Ingestion Service (Go)     │    Amazon ALB
    │   validate · UUID · publish  │
    └──────────────┬───────────────┘
                   ↓
            [ Kafka / MSK ]
                   ↓
    ┌──────────────────────────────┐
    │   Routing Decision Service   │    ECS Fargate · horizontally scaled
    │   circuit breaker · bulkhead │
    │   retry · fail-fast          │
    └─┬────────────┬───────────────┬
      ↓            ↓               ↓
  Warehouse A  Warehouse B    Warehouse C
  ElastiCache  ElastiCache    ElastiCache
  (US-EAST)    (US-CENTRAL)   (US-WEST)
      ↓            ↓               ↓
    ┌──────────────────────────────┐
    │   PostgreSQL Audit Log / RDS │    (routing decisions)
    └──────────────────────────────┘
                   ↓
           [ SQS Dead Letter ]     (failed routings)
```

### Tech Stack

| Layer | Local | Production (AWS) |
|---|---|---|
| HTTP + validation | Go + gorilla/mux | ECS Fargate + ALB |
| Messaging | Redpanda | Amazon MSK |
| State store (×3) | Redis | Amazon ElastiCache |
| Audit log | PostgreSQL | Amazon RDS |
| Dead-letter queue | n/a | Amazon SQS |
| Infrastructure | Docker Compose | Terraform |
| Load testing | Locust | Locust |
| Metrics | Prometheus + Grafana | CloudWatch + Grafana |

---

## Quick Start

### Option 1: Just See The Dashboard (30 seconds)
```bash
start dashboard/index.html   # Windows
```
No backend required. Fully self-contained simulator.
- Click **START LOAD** → watch particles flow
- Click **CRASH WAREHOUSE B** → watch CB open, DLQ rise, auto-failover
- Click **EXP 2** → full automated resilience test

### Option 2: Full Local Stack (2 minutes)
```bash
docker-compose up -d
bash scripts/init_db.sh
cd routing-service && go run main.go   # in one terminal
cd ingestion-service && go run main.go # in another
bash scripts/smoke_test.sh

# View live metrics
start http://localhost:3000   # Grafana (admin/admin)
start http://localhost:9090   # Prometheus
start http://localhost:8080   # Redpanda Console
```

### Option 3: Deploy to AWS (~15 min)
```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars with your values
terraform init
terraform apply

# Run experiments against deployed infrastructure
ALB_HOST=<your-alb-dns> bash scripts/run_experiment1.sh

# When done:
terraform destroy
```

---

## Running Experiments

```bash
# Original 3 experiments
bash scripts/run_experiment1.sh    # Horizontal scaling
bash scripts/run_experiment2.sh    # Resilience under failure
bash scripts/run_experiment3.sh    # Inventory contention

# 4 new experiments
bash scripts/run_experiment4.sh    # Network partition
bash scripts/run_experiment5.sh    # Kafka partitions
bash scripts/run_experiment6.sh    # Noisy neighbor
bash scripts/run_experiment7.sh    # Cold start

# Generate all charts
python scripts/generate_charts.py
python scripts/generate_charts_v2.py
```

---

## Key Findings

### 1. Atomic operations are the real correctness guarantee (Experiment 3)
Zero oversells in both optimistic AND pessimistic strategies. The Lua atomic decrement in Redis provides correctness independently of client-side locking. Optimistic gives 2.3x higher throughput; pessimistic gives lower latency variance.

### 2. Kafka partition count is the real scaling ceiling (Experiment 5)
Adding ECS tasks beyond partition count is waste — consumers sit idle. 3-partition topic caps useful scaling at 3 tasks. Bumping to 12 partitions unlocks 4x throughput headroom.

### 3. Shared resources create hidden tail latency coupling (Experiment 6)
A burst on SKU-HOTITEM caused 4x P99 spike on SKU-ALPHA (completely unrelated). Both share ElastiCache. Multi-tenant workloads need resource isolation.

### 4. Cold start is real, even for Go (Experiment 7)
New ECS tasks peak at 178ms P99 vs 42ms for warm. Dominant cost is Kafka consumer rebalance (~3s) plus Redis pool warm-up (~10s).

### 5. Circuit breakers transform failure behavior
With slow-but-not-crashed Warehouse B:
- **Without CB:** P99 → 2,100ms, success rate 87%, unbounded goroutines
- **With CB:** P99 stays 42ms, success rate 99.8%, bounded resources

### 6. Eventual consistency is fine for WMS (Experiment 4)
120-second partition produced 724-unit drift, reconciled in 20 seconds. Zero orders lost. AP over CP is correct for this domain.

### 7. Failover faster than recovery (Experiment 2)
Failover: 3.2s. Recovery: 6.1s. The extra time is inventory rehydration — worth the cost.

---

## Project Structure

```
warehouseFlow/
├── ingestion-service/              Go HTTP service → Kafka
│   ├── handlers/                   Request validation + Kafka publish
│   ├── kafka/                      Kafka producer wrapper
│   └── main.go
├── routing-service/                Go Kafka consumer → warehouse routing
│   ├── consumer/                   At-least-once Kafka consumer
│   ├── router/                     Routing engines (base/optimistic/pessimistic)
│   ├── store/                      Redis + Postgres + distributed lock
│   ├── resilience/                 5 production patterns + tests
│   ├── dlq/                        SQS dead-letter queue
│   └── main.go
├── dashboard/                      Interactive operations control center
│   ├── index.html · styles.css · app.js
│   └── README.md
├── load-tests/                     7 Locust files (one per experiment)
├── terraform/                      Full AWS infrastructure-as-code
├── monitoring/                     Prometheus + Grafana dashboard
├── scripts/                        Runners + utilities (11 files)
├── results/                        CSVs + PNG charts for all experiments
│   ├── experiment1/ ... experiment7/
│   └── charts/
├── docs/                           Week analyses, final report, guides
├── docker-compose.yml
├── CHANGELOG.md
└── README.md (this file)
```

---

## Documentation

- `docs/week2_analysis.md` — Experiment 1 findings
- `docs/week3_analysis.md` — Experiments 2, 3 findings
- `docs/week4_analysis.md` — Experiments 4, 5, 6, 7 + resilience patterns
- `docs/final_report.md` — Complete academic writeup

---

*CS 6650 Distributed Systems · Solo Project · Nithish Kadam · April 2026*
