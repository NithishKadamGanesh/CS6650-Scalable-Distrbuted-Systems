"""
HW8 Step II — Eventual Consistency Investigation
=================================================
Tests DynamoDB's eventual consistency behavior by:
  1. Create-then-immediately-read (read-after-write)
  2. Add-item-then-immediately-read
  3. Rapid sequential updates to the same cart

Usage:
  python consistency_test.py --host http://<ECS_PUBLIC_IP>:8080
"""

import argparse
import json
import time
from datetime import datetime, timezone
from urllib import request, error

BASE_PREFIX = "/dynamo"

def make_request(method, url, body=None):
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


def test_read_after_write(base_url, iterations=20):
    """Test 1: Create a cart, then immediately read it."""
    print(f"\n{'─'*60}")
    print(f"  Test 1: Read-After-Write Consistency ({iterations} iterations)")
    print(f"{'─'*60}")

    results = []
    for i in range(iterations):
        # Create
        status, body, create_ms = make_request(
            "POST",
            f"{base_url}{BASE_PREFIX}/shopping-carts",
            {"customer_id": 9000 + i}
        )
        cart_id = body.get("shopping_cart_id", -1)

        # Immediately read (no delay)
        status2, body2, read_ms = make_request(
            "GET",
            f"{base_url}{BASE_PREFIX}/shopping-carts/{cart_id}"
        )

        found = status2 == 200
        results.append({
            "iteration": i + 1,
            "cart_id": cart_id,
            "create_ms": round(create_ms, 2),
            "read_ms": round(read_ms, 2),
            "found_immediately": found,
            "read_status": status2
        })

        symbol = "✓" if found else "✗ NOT FOUND"
        print(f"  [{i+1:2d}] Create: {create_ms:.1f}ms → Read: {read_ms:.1f}ms  {symbol}")

    found_count = sum(1 for r in results if r["found_immediately"])
    print(f"\n  Result: {found_count}/{iterations} found immediately after write")
    miss_rate = (iterations - found_count) / iterations * 100
    print(f"  Eventual consistency miss rate: {miss_rate:.1f}%")
    return results


def test_add_then_read(base_url, iterations=20):
    """Test 2: Add item to cart, then immediately read and check item is present."""
    print(f"\n{'─'*60}")
    print(f"  Test 2: Add-Item-Then-Read Consistency ({iterations} iterations)")
    print(f"{'─'*60}")

    # Create a single cart for this test
    status, body, _ = make_request(
        "POST",
        f"{base_url}{BASE_PREFIX}/shopping-carts",
        {"customer_id": 8000}
    )
    cart_id = body.get("shopping_cart_id", -1)
    print(f"  Using cart_id: {cart_id}")

    # Small delay to ensure cart is visible
    time.sleep(0.1)

    results = []
    for i in range(iterations):
        product_id = 5000 + i  # Unique product each time

        # Add item
        status, _, add_ms = make_request(
            "POST",
            f"{base_url}{BASE_PREFIX}/shopping-carts/{cart_id}/items",
            {"product_id": product_id, "quantity": 1}
        )

        # Immediately read
        status2, body2, read_ms = make_request(
            "GET",
            f"{base_url}{BASE_PREFIX}/shopping-carts/{cart_id}"
        )

        # Check if the just-added product appears in items
        items = body2.get("items", [])
        product_ids_in_cart = [item["product_id"] for item in items]
        item_visible = product_id in product_ids_in_cart

        results.append({
            "iteration": i + 1,
            "product_id": product_id,
            "add_ms": round(add_ms, 2),
            "read_ms": round(read_ms, 2),
            "item_visible": item_visible,
            "items_count": len(items)
        })

        symbol = "✓" if item_visible else "✗ NOT VISIBLE"
        print(f"  [{i+1:2d}] Add: {add_ms:.1f}ms → Read: {read_ms:.1f}ms  items={len(items)}  {symbol}")

    visible_count = sum(1 for r in results if r["item_visible"])
    print(f"\n  Result: {visible_count}/{iterations} items visible immediately after add")
    miss_rate = (iterations - visible_count) / iterations * 100
    print(f"  Eventual consistency miss rate: {miss_rate:.1f}%")
    return results


def test_rapid_updates(base_url, iterations=10):
    """Test 3: Rapid sequential updates to the same cart."""
    print(f"\n{'─'*60}")
    print(f"  Test 3: Rapid Sequential Updates ({iterations} updates)")
    print(f"{'─'*60}")

    # Create a cart
    status, body, _ = make_request(
        "POST",
        f"{base_url}{BASE_PREFIX}/shopping-carts",
        {"customer_id": 7000}
    )
    cart_id = body.get("shopping_cart_id", -1)
    print(f"  Using cart_id: {cart_id}")
    time.sleep(0.1)

    # Rapid-fire add the same product repeatedly (quantity += 1 each time)
    results = []
    for i in range(iterations):
        status, _, add_ms = make_request(
            "POST",
            f"{base_url}{BASE_PREFIX}/shopping-carts/{cart_id}/items",
            {"product_id": 9999, "quantity": 1}
        )
        results.append({"iteration": i + 1, "add_ms": round(add_ms, 2), "status": status})
        print(f"  [{i+1:2d}] Add quantity +1: {add_ms:.1f}ms (status: {status})")

    # Final read — check accumulated quantity
    time.sleep(0.5)  # Brief pause before final read
    status, body, read_ms = make_request(
        "GET",
        f"{base_url}{BASE_PREFIX}/shopping-carts/{cart_id}"
    )
    items = body.get("items", [])
    final_qty = 0
    for item in items:
        if item.get("product_id") == 9999:
            final_qty = item.get("quantity", 0)
            break

    print(f"\n  Final read: {read_ms:.1f}ms")
    print(f"  Expected quantity: {iterations}")
    print(f"  Actual quantity:   {final_qty}")
    if final_qty == iterations:
        print(f"  ✓ All updates applied correctly")
    else:
        print(f"  ✗ Lost {iterations - final_qty} updates (race condition)")

    return {
        "cart_id": cart_id,
        "expected_quantity": iterations,
        "actual_quantity": final_qty,
        "updates_lost": iterations - final_qty,
        "update_results": results
    }


def main():
    parser = argparse.ArgumentParser(description="HW8 DynamoDB Consistency Test")
    parser.add_argument("--host", required=True,
                        help="Base URL (e.g., http://1.2.3.4:8080)")
    args = parser.parse_args()
    base_url = args.host.rstrip("/")

    print(f"\n{'='*60}")
    print(f"  HW8 Eventual Consistency Investigation")
    print(f"  Target: {base_url}")
    print(f"  Time:   {datetime.now(timezone.utc).isoformat()}")
    print(f"{'='*60}")

    t1 = test_read_after_write(base_url)
    t2 = test_add_then_read(base_url)
    t3 = test_rapid_updates(base_url)

    # Save all results
    all_results = {
        "test_1_read_after_write": t1,
        "test_2_add_then_read": t2,
        "test_3_rapid_updates": t3,
        "timestamp": datetime.now(timezone.utc).isoformat()
    }

    output_file = "consistency_test_results.json"
    with open(output_file, "w") as f:
        json.dump(all_results, f, indent=2)

    print(f"\n{'='*60}")
    print(f"  All results saved to {output_file}")
    print(f"{'='*60}")


if __name__ == "__main__":
    main()
