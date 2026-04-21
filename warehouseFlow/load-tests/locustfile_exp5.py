"""locustfile_exp5.py — Kafka partition ↔ consumer parallelism matrix."""
from locust import HttpUser, between, task, events
import random, uuid

SKUS = ["SKU-ALPHA", "SKU-BETA", "SKU-GAMMA"]

@events.test_start.add_listener
def on_start(environment, **kwargs):
    print("[Exp5] Kafka parallelism test — N consumers can only consume N partitions")

class Exp5User(HttpUser):
    wait_time = between(0.005, 0.015)

    @task
    def submit(self):
        payload = {
            "customer_id": f"exp5-{uuid.uuid4()}",
            "sku": random.choice(SKUS),
            "quantity": 1,
            "region": random.choice(["US-EAST", "US-CENTRAL", "US-WEST"]),
        }
        self.client.post("/api/v1/orders", json=payload, name="/api/v1/orders")
