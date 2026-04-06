"""
Load Testing for Async Order Processing Assignment
====================================================

Run from the loadtest/ directory:

    pip install locust

Phase 1 — Normal Operations (sync, 5 users):
    locust -f locustfile.py SyncUser \
        --host http://<ALB_DNS> \
        --users 5 --spawn-rate 1 --run-time 30s \
        --headless --csv results/sync_normal

Phase 1 — Flash Sale (sync, 20 users):
    locust -f locustfile.py SyncUser \
        --host http://<ALB_DNS> \
        --users 20 --spawn-rate 10 --run-time 60s \
        --headless --csv results/sync_flash

Phase 3 — Flash Sale (async, 20 users):
    locust -f locustfile.py AsyncUser \
        --host http://<ALB_DNS> \
        --users 20 --spawn-rate 10 --run-time 60s \
        --headless --csv results/async_flash

Phase 5 — Scaled workers (async, 20 users):
    (Change WORKER_COUNT env var on the processor, then re-run the async test)

Combined test (both user types simultaneously):
    locust -f locustfile.py \
        --host http://<ALB_DNS> \
        --users 20 --spawn-rate 10 --run-time 60s
"""

import json
import random
import time
from locust import HttpUser, task, between, events


def generate_order_payload():
    """Generate a realistic-looking order request."""
    num_items = random.randint(1, 5)
    items = []
    products = [
        ("PROD-001", "Wireless Mouse", 29.99),
        ("PROD-002", "Mechanical Keyboard", 89.99),
        ("PROD-003", "USB-C Hub", 45.99),
        ("PROD-004", "Monitor Stand", 34.99),
        ("PROD-005", "Webcam HD", 59.99),
        ("PROD-006", "Desk Lamp", 24.99),
        ("PROD-007", "Mouse Pad XL", 19.99),
        ("PROD-008", "Cable Organizer", 12.99),
    ]

    for _ in range(num_items):
        prod_id, name, price = random.choice(products)
        items.append({
            "product_id": prod_id,
            "name": name,
            "quantity": random.randint(1, 3),
            "price": price,
        })

    return {
        "customer_id": random.randint(1000, 9999),
        "items": items,
    }


class SyncUser(HttpUser):
    """
    Simulates a customer hitting the synchronous endpoint.

    Phase 1 — Normal: 5 users, spawn rate 1/s, 30 seconds
    Phase 1 — Flash:  20 users, spawn rate 10/s, 60 seconds

    Wait time: random 100-500ms between requests (per assignment spec)
    """

    wait_time = between(0.1, 0.5)  # 100-500ms between requests

    @task
    def place_order_sync(self):
        payload = generate_order_payload()
        with self.client.post(
            "/orders/sync",
            json=payload,
            catch_response=True,
            timeout=30,  # 30s timeout to capture slow responses
            name="POST /orders/sync",
        ) as response:
            if response.status_code == 200:
                response.success()
            else:
                response.failure(
                    f"Status {response.status_code}: {response.text[:200]}"
                )


class AsyncUser(HttpUser):
    """
    Simulates a customer hitting the asynchronous endpoint.

    Phase 3 — Flash: 20 users, spawn rate 10/s, 60 seconds

    Same wait time as SyncUser for fair comparison.
    """

    wait_time = between(0.1, 0.5)  # 100-500ms between requests

    @task
    def place_order_async(self):
        payload = generate_order_payload()
        with self.client.post(
            "/orders/async",
            json=payload,
            catch_response=True,
            timeout=10,
            name="POST /orders/async",
        ) as response:
            if response.status_code == 202:
                response.success()
            else:
                response.failure(
                    f"Status {response.status_code}: {response.text[:200]}"
                )


class MixedUser(HttpUser):
    """
    Sends both sync and async requests for side-by-side comparison.
    Useful for seeing both endpoints under identical conditions.
    """

    wait_time = between(0.1, 0.5)

    @task(1)
    def place_order_sync(self):
        payload = generate_order_payload()
        self.client.post(
            "/orders/sync",
            json=payload,
            timeout=30,
            name="POST /orders/sync",
        )

    @task(1)
    def place_order_async(self):
        payload = generate_order_payload()
        self.client.post(
            "/orders/async",
            json=payload,
            timeout=10,
            name="POST /orders/async",
        )
