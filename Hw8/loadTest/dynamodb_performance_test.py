"""
HW8 Step II Performance Test — 150 Operations Against DynamoDB-Backed Shopping Cart
====================================================================================
IDENTICAL to Step I test: 50 create, 50 add items, 50 get cart.
Results saved to dynamodb_test_results.json (same format as mysql_test_results.json).

Usage:
  python dynamodb_performance_test.py --host http://<ECS_PUBLIC_IP>:8080
"""

import argparse
import json
import random
import time
from datetime import datetime, timezone
from urllib import request, error

# DynamoDB endpoints use /dynamo/ prefix
BASE_PREFIX = "/dynamo"

def make_request(method, url, body=None):
    """Send HTTP request and return (status, response_body, elapsed_ms)."""
    data = json.dumps(body).encode("utf-8") if body else None
    headers = {"Content-Type": "application/json"} if body else {}

    req = request.Request(url, data=data, headers=headers, method=method)

    start = time.perf_counter()
    try:
        with request.urlopen(req) as resp:
            elapsed = (time.perf_counter() - start) * 1000
            raw = resp.read().decode("utf-8").strip()
            resp_body = json.loads(raw) if raw else {}
            return resp.status, resp_body, elapsed
    except error.HTTPError as e:
        elapsed = (time.perf_counter() - start) * 1000
        resp_body = {}
        try:
            raw = e.read().decode("utf-8").strip()
            if raw:
                resp_body = json.loads(raw)
        except Exception:
            pass
        return e.code, resp_body, elapsed

def run_test(base_url):
    results = []
    cart_ids = []

    print(f"\n{'='*60}")
    print(f"  HW8 DynamoDB Performance Test (Step II)")
    print(f"  Target: {base_url}")
    print(f"  Time:   {datetime.now(timezone.utc).isoformat()}")
    print(f"{'='*60}\n")

    # ── Phase 1: Create 50 carts ─────────────────────────────
    print("[Phase 1] Creating 50 shopping carts (DynamoDB)...")
    for i in range(50):
        customer_id = 2000 + i  # Different range from MySQL test
        status, body, elapsed = make_request(
            "POST",
            f"{base_url}{BASE_PREFIX}/shopping-carts",
            {"customer_id": customer_id}
        )
        success = status == 201
        if success and "shopping_cart_id" in body:
            cart_ids.append(body["shopping_cart_id"])

        results.append({
            "operation": "create_cart",
            "response_time": round(elapsed, 2),
            "success": success,
            "status_code": status,
            "timestamp": datetime.now(timezone.utc).isoformat()
        })

        if (i + 1) % 10 == 0:
            print(f"  Created {i+1}/50  (last: {elapsed:.1f}ms, status: {status})")

    print(f"  ✓ {len(cart_ids)} carts created successfully\n")

    # ── Phase 2: Add items to 50 carts ───────────────────────
    print("[Phase 2] Adding items to 50 carts (DynamoDB)...")
    for i in range(50):
        if not cart_ids:
            print("  ✗ No carts available to add items to!")
            break

        cart_id = cart_ids[i % len(cart_ids)]
        product_id = random.randint(1, 1000)
        quantity = random.randint(1, 5)

        status, body, elapsed = make_request(
            "POST",
            f"{base_url}{BASE_PREFIX}/shopping-carts/{cart_id}/items",
            {"product_id": product_id, "quantity": quantity}
        )

        results.append({
            "operation": "add_items",
            "response_time": round(elapsed, 2),
            "success": status == 204,
            "status_code": status,
            "timestamp": datetime.now(timezone.utc).isoformat()
        })

        if (i + 1) % 10 == 0:
            print(f"  Added {i+1}/50   (last: {elapsed:.1f}ms, status: {status})")

    print(f"  ✓ Item additions complete\n")

    # ── Phase 3: Retrieve 50 carts ───────────────────────────
    print("[Phase 3] Retrieving 50 carts (DynamoDB)...")
    for i in range(50):
        if not cart_ids:
            print("  ✗ No carts available to retrieve!")
            break

        cart_id = cart_ids[i % len(cart_ids)]

        status, body, elapsed = make_request(
            "GET",
            f"{base_url}{BASE_PREFIX}/shopping-carts/{cart_id}"
        )

        results.append({
            "operation": "get_cart",
            "response_time": round(elapsed, 2),
            "success": status == 200,
            "status_code": status,
            "timestamp": datetime.now(timezone.utc).isoformat()
        })

        if (i + 1) % 10 == 0:
            print(f"  Retrieved {i+1}/50 (last: {elapsed:.1f}ms, status: {status})")

    print(f"  ✓ Retrievals complete\n")

    # ── Summary ──────────────────────────────────────────────
    print(f"{'='*60}")
    print(f"  RESULTS SUMMARY (DynamoDB)")
    print(f"{'='*60}")

    for op in ["create_cart", "add_items", "get_cart"]:
        op_results = [r for r in results if r["operation"] == op]
        successes = sum(1 for r in op_results if r["success"])
        times = [r["response_time"] for r in op_results]
        avg_time = sum(times) / len(times) if times else 0
        min_time = min(times) if times else 0
        max_time = max(times) if times else 0

        sorted_times = sorted(times)
        p95_idx = int(len(sorted_times) * 0.95)
        p95_time = sorted_times[min(p95_idx, len(sorted_times) - 1)] if sorted_times else 0

        print(f"\n  {op}:")
        print(f"    Success: {successes}/{len(op_results)}")
        print(f"    Avg:     {avg_time:.1f}ms")
        print(f"    Min:     {min_time:.1f}ms")
        print(f"    Max:     {max_time:.1f}ms")
        print(f"    P95:     {p95_time:.1f}ms")

    total_success = sum(1 for r in results if r["success"])
    print(f"\n  TOTAL: {total_success}/{len(results)} successful")
    print(f"{'='*60}\n")

    # ── Save results ─────────────────────────────────────────
    output_file = "dynamodb_test_results.json"
    with open(output_file, "w") as f:
        json.dump(results, f, indent=2)

    print(f"Results saved to {output_file}")
    return results

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="HW8 DynamoDB Performance Test")
    parser.add_argument("--host", required=True,
                        help="Base URL (e.g., http://1.2.3.4:8080)")
    args = parser.parse_args()

    base_url = args.host.rstrip("/")

    start_time = time.time()
    run_test(base_url)
    total_time = time.time() - start_time

    print(f"\nTotal test duration: {total_time:.1f}s")
    if total_time > 300:
        print("⚠  WARNING: Test exceeded the 5-minute window!")
    else:
        print(f"✓  Completed within 5-minute window ({total_time:.0f}s / 300s)")
