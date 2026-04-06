"""
HW8 Locust Load Test — DynamoDB Shopping Cart API
===================================================
Usage:
  locust -f locust_dynamo_cart.py --host http://<ECS_PUBLIC_IP>:8080
"""

import random
import itertools
from locust import HttpUser, task, between

global_customer_counter = itertools.count(start=300000)

class DynamoShoppingCartUser(HttpUser):
    wait_time = between(0.1, 0.5)

    def on_start(self):
        self.cart_ids = []

    @task(3)
    def create_cart(self):
        customer_id = next(global_customer_counter)
        resp = self.client.post(
            "/dynamo/shopping-carts",
            json={"customer_id": customer_id},
            name="/dynamo/shopping-carts POST"
        )
        if resp.status_code == 201:
            body = resp.json()
            if "shopping_cart_id" in body:
                self.cart_ids.append(body["shopping_cart_id"])

    @task(4)
    def add_item_to_cart(self):
        if not self.cart_ids:
            return
        cart_id = random.choice(self.cart_ids)
        product_id = random.randint(1, 5000)
        quantity = random.randint(1, 3)
        self.client.post(
            f"/dynamo/shopping-carts/{cart_id}/items",
            json={"product_id": product_id, "quantity": quantity},
            name="/dynamo/shopping-carts/[id]/items POST"
        )

    @task(3)
    def get_cart(self):
        if not self.cart_ids:
            return
        cart_id = random.choice(self.cart_ids)
        self.client.get(
            f"/dynamo/shopping-carts/{cart_id}",
            name="/dynamo/shopping-carts/[id] GET"
        )
