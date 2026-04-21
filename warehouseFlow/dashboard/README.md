# WarehouseFlow Operations Control

Standalone HTML/JS/CSS dashboard that visualizes the WarehouseFlow system in real time.

## What It Shows

- **Warehouse network map** — 3 warehouses with live inventory, picker count, circuit breaker state
- **Live routing particles** — color-coded dots flowing from ingestion → router → warehouse
- **KPI strip** — throughput, P95/P99 latency, success rate, DLQ depth, oversells
- **4 time-series charts** — throughput, latency (P50/P95/P99), per-warehouse stacked, counters
- **Live order feed** — last 40 orders with latency and destination
- **System event ticker** — circuit breaker transitions, failures, recoveries
- **7-experiment autopilot** — one-click automated scenarios

## How To Run

No backend required. Self-contained simulator.

```bash
# Windows
start index.html

# Or just double-click the file
```

Best results at 1400px+ screen width.

## 2-Minute Demo Script

1. Click **START LOAD** — watch particles flow
2. Click **CRASH WAREHOUSE B** — watch CB flip to OPEN, DLQ rise, auto-degrade
3. Click **RECOVER ALL** — watch CB transition CLOSED → HALF-OPEN → CLOSED
4. Toggle **CIRCUIT BREAKER off**, crash B again — watch latency climb unbounded
5. Click **EXP 3** — burst of 1000 orders for SKU-HOTITEM, zero oversells

## Design

- **Industrial operations control center** aesthetic
- **Signal colors only** — no decorative color
- **Vanilla HTML/CSS/JS** — zero dependencies
- **Canvas for charts**, **SVG for map**, **requestAnimationFrame for particles**
- **60fps rendering**
- **No backend calls** — fully self-contained

## Files

```
dashboard/
├── index.html   Layout + SVG warehouse map
├── styles.css   Dark industrial theme
├── app.js       Simulator + charts + particles + experiment autopilot
└── README.md    This file
```
