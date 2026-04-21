"""locustfile_exp1.py — Experiment 1 steady load at 5,000 orders/min."""
from locust import HttpUser, between, task, events
import random, uuid, time

SKUS = ["SKU-ALPHA", "SKU-BETA", "SKU-GAMMA"]
REGIONS = ["US-EAST", "US-CENTRAL", "US-WEST"]

_start = None

@events.test_start.add_listener
def on_start(environment, **kwargs):
    global _start
    _start = time.time()
    print("[Exp1] Starting steady 5000 orders/min test")

@events.test_stop.add_listener
def on_stop(environment, **kwargs):
    stats = environment.stats.get("/api/v1/orders", "POST")
    print(f"[Exp1] Summary: {stats.num_requests} req, {stats.num_failures} fail, "
          f"RPS={stats.current_rps:.1f}, P95={stats.get_response_time_percentile(0.95):.1f}ms, "
          f"P99={stats.get_response_time_percentile(0.99):.1f}ms")

class Exp1User(HttpUser):
    wait_time = between(0.005, 0.015)

    @task
    def submit(self):
        payload = {
            "customer_id": f"exp1-{uuid.uuid4()}",
            "sku": random.choice(SKUS),
            "quantity": 1,
            "region": random.choice(REGIONS),
        }
        with self.client.post("/api/v1/orders", json=payload,
                              name="/api/v1/orders", catch_response=True) as r:
            if r.status_code == 202: r.success()
            else: r.failure(f"status {r.status_code}")
