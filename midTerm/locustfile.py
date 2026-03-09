"""
Galactic Pizza Ordering Service — Locust Load Test
CS 6650 Step III

Uses FastHttpUser to minimize client-side overhead (same as HW6).

Local:
  locust -f locustfile.py --host=http://localhost:8080

AWS:
  locust -f locustfile.py --host=http://<ALB-DNS-NAME>

Headless (for metrics collection):
  locust -f locustfile.py --host=http://localhost:8080 \
    --users 50 --spawn-rate 10 -t 60s --headless

With web UI:
  locust -f locustfile.py --host=http://localhost:8080
  # Open http://localhost:8089
"""

from locust import FastHttpUser, task, between
import json


class PizzaOrderUser(FastHttpUser):
    """Simulates customers placing pizza orders."""
    wait_time = between(0.1, 0.5)

    @task(10)
    def place_order(self):
        with self.client.get("/order", catch_response=True) as resp:
            if resp.status_code == 200:
                resp.success()
            elif resp.status_code == 503:
                body = json.loads(resp.text)
                resp.failure(f"Degraded: {body.get('error', 'unknown')}")
            else:
                resp.failure(f"Unexpected: {resp.status_code}")

    @task(1)
    def check_health(self):
        self.client.get("/health")


class MetricsCollector(FastHttpUser):
    """Polls /metrics to correlate server-side and client-side views."""
    wait_time = between(2, 5)
    fixed_count = 1

    @task
    def collect_metrics(self):
        with self.client.get("/metrics", catch_response=True) as resp:
            if resp.status_code == 200:
                d = json.loads(resp.text)
                print(
                    f"[Server] "
                    f"sr={d.get('success_rate_pct',0):.1f}% "
                    f"p95={d.get('latency_p95_ms',0):.0f}ms "
                    f"p99={d.get('latency_p99_ms',0):.0f}ms "
                    f"cb_trips={d.get('circuit_trips',0)} "
                    f"bh_rejects={d.get('bulkhead_rejects',0)}"
                )
                resp.success()
