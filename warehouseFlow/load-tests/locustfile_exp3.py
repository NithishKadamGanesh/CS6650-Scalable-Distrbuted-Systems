"""locustfile_exp3.py — 1000 concurrent orders for SKU-HOTITEM (100 units)."""
from locust import HttpUser, constant, task, events
import uuid

_results = {"routed": 0, "rejected": 0, "errors": 0}

@events.request.add_listener
def on_request(request_type, name, response_time, response_length,
               response, context, exception, **kwargs):
    if exception: _results["errors"] += 1
    elif response and response.status_code == 202: _results["routed"] += 1
    elif response and response.status_code in (409, 503): _results["rejected"] += 1

@events.test_stop.add_listener
def on_stop(environment, **kwargs):
    stats = environment.stats.get("/api/v1/orders [scarcity]", "POST")
    print(f"\n[Exp3] Results:")
    print(f"  202 (routed)    : {_results['routed']}")
    print(f"  409/503 (reject): {_results['rejected']}")
    print(f"  Errors          : {_results['errors']}")
    print(f"  P99 latency     : {stats.get_response_time_percentile(0.99):.1f}ms")
    print(f"  Throughput      : {stats.current_rps:.1f} req/s")
    print(f"\n  Verify oversells: SELECT COUNT(*) FROM routing_decisions")
    print(f"  WHERE sku='SKU-HOTITEM' AND status='routed'; -- should be exactly 100")

class Exp3User(HttpUser):
    wait_time = constant(0)

    @task
    def submit(self):
        payload = {
            "customer_id": f"exp3-{uuid.uuid4()}",
            "sku": "SKU-HOTITEM",
            "quantity": 1,
            "region": "US-EAST",
        }
        with self.client.post("/api/v1/orders", json=payload,
                              name="/api/v1/orders [scarcity]", catch_response=True) as r:
            if r.status_code == 202: r.success()
            elif r.status_code in (409, 503): r.success()  # expected rejection
            else: r.failure(f"status {r.status_code}")
