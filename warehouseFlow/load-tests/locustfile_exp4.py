"""locustfile_exp4.py — Network partition CAP test."""
from locust import HttpUser, between, task, events
import random, uuid

SKUS = ["SKU-ALPHA", "SKU-BETA", "SKU-GAMMA"]

@events.test_start.add_listener
def on_start(environment, **kwargs):
    print("[Exp4] Partition test — expected: 0 orders lost, eventual consistency")

class Exp4User(HttpUser):
    wait_time = between(0.01, 0.03)

    @task
    def submit(self):
        payload = {
            "customer_id": f"exp4-{uuid.uuid4()}",
            "sku": random.choice(SKUS),
            "quantity": 1,
            "region": random.choice(["US-EAST", "US-CENTRAL", "US-WEST"]),
        }
        with self.client.post("/api/v1/orders", json=payload,
                              name="submit_order", catch_response=True) as r:
            if r.status_code in (200, 202, 503): r.success()
            else: r.failure(f"status {r.status_code}")
