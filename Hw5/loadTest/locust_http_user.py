import random
import itertools
from locust import HttpUser, task, between

# Global shared counter across ALL users
global_id_counter = itertools.count(start=100000)

class ProductHttpUser(HttpUser):

    wait_time = between(0.1, 0.5)

    def on_start(self):
        # Each user keeps track of IDs it created
        self.my_products = []

    # 40% CREATE
    @task(4)
    def create_product(self):
        product_id = next(global_id_counter)

        self.my_products.append(product_id)

        self.client.post(
            "/products",
            json={
                "product_id": product_id,
                "sku": f"SKU-{product_id}",
                "manufacturer": "HttpUser",
                "category_id": 1,
                "weight": 100,
                "some_other_id": 1
            },
            name="/products POST"
        )

    # 40% GET
    @task(4)
    def get_product(self):
        if not self.my_products:
            return

        product_id = random.choice(self.my_products)

        self.client.get(
            f"/products/{product_id}",
            name="/products/[id] GET"
        )

    # 20% UPDATE
    @task(2)
    def update_product(self):
        if not self.my_products:
            return

        product_id = random.choice(self.my_products)

        self.client.post(
            f"/products/{product_id}/details",
            json={
                "product_id": product_id,
                "sku": f"SKU-{product_id}-UPDATED",
                "manufacturer": "Updated",
                "category_id": 2,
                "weight": 150,
                "some_other_id": 2
            },
            name="/products/[id]/details POST"
        )
