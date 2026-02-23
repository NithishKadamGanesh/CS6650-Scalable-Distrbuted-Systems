from locust import FastHttpUser, task

class SearchUser(FastHttpUser):

    @task
    def search(self):
        self.client.get("/products/search?q=Electronics")