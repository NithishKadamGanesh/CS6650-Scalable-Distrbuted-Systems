"""locustfile_exp6.py — Noisy neighbor tail latency test."""
from locust import HttpUser, between, task, events
import random, uuid, time

_start = None

@events.test_start.add_listener
def on_start(environment, **kwargs):
    global _start
    _start = time.time()
    print("[Exp6] Baseline + noisy neighbor test")
    print("[Exp6] SKU-HOTITEM burst activates at t=60s")

class QuietUser(HttpUser):
    """Baseline SKU-ALPHA at low rate."""
    weight = 3
    wait_time = between(0.03, 0.05)

    @task
    def submit_alpha(self):
        payload = {
            "customer_id": f"quiet-{uuid.uuid4()}",
            "sku": "SKU-ALPHA",
            "quantity": 1,
            "region": random.choice(["US-EAST", "US-CENTRAL", "US-WEST"]),
        }
        self.client.post("/api/v1/orders", json=payload, name="alpha_order")

class NoisyNeighbor(HttpUser):
    """SKU-HOTITEM burst — activates at t=60s."""
    weight = 1
    wait_time = between(0.001, 0.003)

    @task
    def submit_hotitem(self):
        if _start is None or time.time() - _start < 60:
            return
        payload = {
            "customer_id": f"noisy-{uuid.uuid4()}",
            "sku": "SKU-HOTITEM",
            "quantity": 1,
            "region": "US-EAST",
        }
        self.client.post("/api/v1/orders", json=payload, name="hotitem_order")
