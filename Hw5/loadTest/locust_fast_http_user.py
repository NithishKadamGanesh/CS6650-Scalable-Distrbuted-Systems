import random
import itertools
from locust import FastHttpUser, task, between

global_id_counter = itertools.count(start=500000)

class ProductFastHttpUser(FastHttpUser):

    wait_time = between(0.1, 0.5)

    def on_start(self):
        self.my_products = []

    @task(4)
    def create_product(self):
        product_id = next(global_id_counter)

        self.my_products.append(product_id)

        self.client.post(
            "/products",
            json={
                "product_id": product_id,
                "sku": f"SKU-{product_id}",
                "manufacturer": "FastUser",
                "category_id": 1,
                "weight": 100,
                "some_other_id": 1
            },
            name="/products POST"
        )

    @task(4)
    def get_product(self):
        if not self.my_products:
            return

        product_id = random.choice(self.my_products)

        self.client.get(
            f"/products/{product_id}",
            name="/products/[id] GET"
        )

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
                "manufacturer": "FastUpdated",
                "category_id": 2,
                "weight": 150,
                "some_other_id": 2
            },
            name="/products/[id]/details POST"
        )
