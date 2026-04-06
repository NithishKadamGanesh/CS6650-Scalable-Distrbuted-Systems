"""
HW8 Locust Load Test — Shopping Cart API (MySQL-backed)
========================================================
Usage:
  locust -f locust_shopping_cart.py --host http://<ECS_PUBLIC_IP>:8080

This creates carts, adds items, and retrieves carts in a 33/33/33 ratio
to simulate realistic shopping behavior under load.
"""

import random
import itertools
from locust import HttpUser, task, between

# Global cart ID counter to avoid collisions across users
global_customer_counter = itertools.count(start=200000)

class ShoppingCartUser(HttpUser):
    wait_time = between(0.1, 0.5)

    def on_start(self):
        """Each user starts by creating a cart."""
        self.cart_ids = []

    @task(3)
    def create_cart(self):
        """POST /shopping-carts — create a new cart."""
        customer_id = next(global_customer_counter)
        resp = self.client.post(
            "/shopping-carts",
            json={"customer_id": customer_id},
            name="/shopping-carts POST"
        )
        if resp.status_code == 201:
            body = resp.json()
            if "shopping_cart_id" in body:
                self.cart_ids.append(body["shopping_cart_id"])

    @task(4)
    def add_item_to_cart(self):
        """POST /shopping-carts/{id}/items — add item to random cart."""
        if not self.cart_ids:
            return

        cart_id = random.choice(self.cart_ids)
        product_id = random.randint(1, 5000)
        quantity = random.randint(1, 3)

        self.client.post(
            f"/shopping-carts/{cart_id}/items",
            json={"product_id": product_id, "quantity": quantity},
            name="/shopping-carts/[id]/items POST"
        )

    @task(3)
    def get_cart(self):
        """GET /shopping-carts/{id} — retrieve cart with items."""
        if not self.cart_ids:
            return

        cart_id = random.choice(self.cart_ids)
        self.client.get(
            f"/shopping-carts/{cart_id}",
            name="/shopping-carts/[id] GET"
        )
