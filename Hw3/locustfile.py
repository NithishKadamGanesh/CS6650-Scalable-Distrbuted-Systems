import uuid
from locust import FastHttpUser, task, between

class AlbumUser(FastHttpUser):
    wait_time = between(1, 2)

    @task
    def get_albums(self):
        self.client.get("/albums")

    @task
    def post_album(self):
        album_id = str(uuid.uuid4())
        self.client.post(
            "/albums",
            json={
                "id": album_id,
                "title": "Locust Album",
                "artist": "Load Tester",
                "price": 99.99
            }
        )
