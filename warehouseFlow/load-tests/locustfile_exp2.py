"""locustfile_exp2.py — Experiment 2 resilience. Failure injected externally."""
from locust import HttpUser, between, task, events
import random, uuid, time, csv, os

SKUS = ["SKU-ALPHA", "SKU-BETA", "SKU-GAMMA"]
REGIONS = ["US-EAST", "US-CENTRAL", "US-WEST"]
_log = []

@events.request.add_listener
def on_request(request_type, name, response_time, response_length,
               response, context, exception, **kwargs):
    _log.append({"ts": time.time(), "latency_ms": response_time,
                 "success": exception is None and (response is None or response.status_code == 202)})

@events.test_stop.add_listener
def on_stop(environment, **kwargs):
    if not _log: return
    os.makedirs("results/experiment2", exist_ok=True)
    out = f"results/experiment2/exp2_persecond_{int(time.time())}.csv"
    start = _log[0]["ts"]

    buckets = {}
    for e in _log:
        sec = int(e["ts"] - start)
        b = buckets.setdefault(sec, {"total": 0, "success": 0, "latencies": []})
        b["total"] += 1
        if e["success"]: b["success"] += 1
        b["latencies"].append(e["latency_ms"])

    with open(out, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["second", "total", "success", "success_rate_pct",
                    "avg_latency_ms", "p95_latency_ms", "p99_latency_ms"])
        for sec in sorted(buckets):
            b = buckets[sec]
            lats = sorted(b["latencies"])
            n = len(lats)
            p95 = lats[int(n * 0.95)] if n else 0
            p99 = lats[int(n * 0.99)] if n else 0
            avg = sum(lats) / n if n else 0
            rate = (b["success"] / b["total"] * 100) if b["total"] else 0
            w.writerow([sec, b["total"], b["success"], f"{rate:.1f}",
                        f"{avg:.1f}", f"{p95:.1f}", f"{p99:.1f}"])
    print(f"[Exp2] Per-second metrics → {out}")

class Exp2User(HttpUser):
    wait_time = between(0.005, 0.015)

    @task
    def submit(self):
        payload = {
            "customer_id": f"exp2-{uuid.uuid4()}",
            "sku": random.choice(SKUS),
            "quantity": 1,
            "region": random.choice(REGIONS),
        }
        with self.client.post("/api/v1/orders", json=payload,
                              name="/api/v1/orders", catch_response=True) as r:
            if r.status_code == 202: r.success()
            elif r.status_code == 503: r.failure("routing_unavailable")
            else: r.failure(f"status_{r.status_code}")
