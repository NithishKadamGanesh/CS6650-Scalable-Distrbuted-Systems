# Hummingbird is Down — Can You Fix It?

## The Scenario

You're a new engineer on the Hummingbird team. It's your first week. The senior engineer is on
vacation. Support tickets are coming in. You have Claude Code, a terminal, and `curl`.

---

## Your Environment

| What              | How to access                                                             |
|------------       |---                                                                        |
| Source code       | Which you cloned in your current directory                                |
| CloudWatch logs   | `aws logs tail /ecs/hummingbird/production/api --follow`                  |
| Claude Code       | `export ANTHROPIC_MODEL='us.anthropic.claude-opus-4-6-v1'` then `claude`  |

> **Start here — confirm the API is up:**
> ```bash
> curl http://<alb-dns>/health
> ```
> You should see `{"status":"ok","service":"hummingbird"}`.

---

## Understand the System First

Before touching any bug, open Claude Code and ask:

```
Give me a high-level walkthrough of this codebase.
What is the full lifecycle of an uploaded image — from POST /upload to GET /download?
Which file and function handles each step?
```

---

## The Tickets

---

### Ticket #1 — "Server started on the wrong port" *(Easy)*

> *"A teammate forgot `APP_PORT` in their `.env` file. The server started — no crash,
> no error — but it wasn't on port 9000. The log said `listening on port undefined`."*

**Investigate with Claude Code:**
```
In server.js, how is the port determined?
What happens in Node.js when you call app.listen(undefined)?
Is there any fallback or guard?
```

**Fix accepted when:** `server.js` uses `process.env.APP_PORT || 9000`.

---

### Ticket #2 — "Width is missing from metadata" *(Easy)*

> *"I upload with `?width=800`. I call `GET /v1/media/:id` straight after.
> The response has `size`, `name`, `status` — but no `width` anywhere."*

**Reproduce it:**
```bash
curl -X POST "http://<alb-dns>/v1/media/upload?width=800" -F "file=@sample.jpg"
curl http://<alb-dns>/v1/media/<mediaId>
```

**Investigate with Claude Code:**
```
In clients/dynamodb.js, what fields does createMedia save to DynamoDB?
Now look at what getMedia returns. Are all the same fields present?
```

**Fix accepted when:** `getMedia` return object includes `width: Number(Item.width.N)`.

---

### Ticket #3 — "Your redirect URL is broken" *(Intermediate)*

> *"I try to download while the image is still processing. I get a `202` — fine.
> But the `Location` header says:*
> ```
> Location: hummingbird-alb-xxx.elb.amazonaws.com/v1/media/abc/status
> ```
> *No `http://`. My client can't follow it."*

**Reproduce it:**
```bash
curl -X POST "http://<alb-dns>/v1/media/upload?width=500" -F "file=@sample.jpg"
curl -i http://<alb-dns>/v1/media/<mediaId>/download
```

Look at the `Location` header in the output.

**Investigate with Claude Code:**
```
In the downloadController in controllers/media.js, show me how the Location header is built.
What does req.hostname return vs req.get('host') in Express?
What does a valid Location header look like for a 3xx response?
```

**Fix accepted when:** Location value starts with `http://` and includes the host and port.

---

### Ticket #4 — "Download never redirects even when COMPLETE" *(Intermediate)*

> *"`GET /status` says `COMPLETE`. `GET /download` still returns `202`. Every time. Forever."*

**Reproduce it:**
```bash
curl -X POST "http://<alb-dns>/v1/media/upload?width=500" -F "file=@sample.jpg"
curl -X PUT "http://<alb-dns>/v1/media/<mediaId>/resize?width=500"
curl http://<alb-dns>/v1/media/<mediaId>/status
curl -i http://<alb-dns>/v1/media/<mediaId>/download
```

Check CloudWatch — the `downloadController` logs the `currentStatus` on every 202:
```bash
aws logs tail /ecs/hummingbird/production/api --follow
```

**Investigate with Claude Code:**
```
In the downloadController in controllers/media.js, what condition decides
whether to return a 202 vs a 302?
Is that comparison correct given that COMPLETE status should trigger the redirect?
```

**Fix accepted when:** The condition checks `media.status !== MEDIA_STATUS.COMPLETE`.

---

## Bonus — "Status never changes. No errors. Nothing." *(Advanced)*

> *"`PUT /resize` responds `{ status: 'COMPLETE' }`. `GET /status` says `PENDING`.
> Resize again — still `PENDING`. Zero errors in the logs. The resize appears to work every time."*

Tail logs while triggering a resize:
```bash
aws logs tail /ecs/hummingbird/production/api --follow &
curl -X PUT "http://<alb-dns>/v1/media/<mediaId>/resize?width=500"
```

Look at **all three** DynamoDB log lines carefully. Pay attention to every field.

**Investigate with Claude Code:**
```
Compare the DynamoDB key used in createMedia, getMedia, and setMediaStatus
in clients/dynamodb.js. Are all three targeting the exact same item?
```

Then ask:
```
What does DynamoDB do when UpdateItem is called on a key that doesn't exist
and there is no ConditionExpression?
```

**Fix accepted when:** `setMediaStatus` uses `SK: { S: 'METADATA' }` — matching the casing in
`createMedia` and `getMedia`.
