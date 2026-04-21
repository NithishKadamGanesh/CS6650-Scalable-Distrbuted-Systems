"""locustfile_exp7.py — Cold start after ECS auto-scale."""
from locust import HttpUser, between, task
import random, uuid

class Exp7User(HttpUser):
    wait_time = between(0.005, 0.015)

    @task
    def submit(self):
        payload = {
            "customer_id": f"exp7-{uuid.uuid4()}",
            "sku": random.choice(["SKU-ALPHA", "SKU-BETA", "SKU-GAMMA"]),
            "quantity": 1,
            "region": random.choice(["US-EAST", "US-CENTRAL", "US-WEST"]),
        }
        self.client.post("/api/v1/orders", json=payload,
                         name="submit_order",
                         headers={"X-Exp-Source": "exp7"})
