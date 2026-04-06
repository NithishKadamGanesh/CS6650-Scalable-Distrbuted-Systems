# HW8 — Complete Execution Guide (Step 3 Onwards)

> **Starting point:** You've already run `go mod tidy` (Step 1) and have Docker Desktop running.
> Your project lives at: `C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8`

---

## STEP 3: Initialize and Deploy Terraform

### 3A: Open a terminal and navigate to the terraform directory

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\terraform-config\terraform"
```

### 3B: Make sure AWS credentials are configured

Run this to verify your credentials are active:

```powershell
aws sts get-caller-identity
```

You should see output with your Account ID, ARN, and UserId. If it fails, re-run:

```powershell
aws configure
```

Enter your **Access Key ID**, **Secret Access Key**, region (`us-east-1`), and output format (`json`).

If you're using a lab/classroom environment with a session token, also run:

```powershell
aws configure set aws_session_token YOUR_SESSION_TOKEN_HERE
```

### 3C: Make sure Docker Desktop is running

Open Docker Desktop on your machine. Wait until the whale icon in the system tray shows "Docker Desktop is running." You can verify from the terminal:

```powershell
docker info
```

If this prints system info without errors, Docker is ready.

### 3D: Initialize Terraform

```powershell
terraform init
```

**What to expect:**
- Downloads the `hashicorp/aws` provider (~6.7.0)
- Downloads the `kreuzwerker/docker` provider (~3.0)
- Discovers 5 modules: ecr, ecs, logging, network, rds
- Final line: `Terraform has been successfully initialized!`

**If you get errors:**
- "Failed to query available provider packages" → check internet connection
- "Module not installed" → make sure you're in the `terraform/` directory, not `terraform-config/`

### 3E: Preview what Terraform will create (optional but recommended)

```powershell
terraform plan -var="db_password=YourSecurePass123!"
```

Replace `YourSecurePass123!` with any password you want (8+ characters, must include uppercase, lowercase, and a number for RDS). **Remember this password — you'll need it for `destroy` later.**

**What to expect:** A list of ~12 resources to create:
- `aws_ecr_repository`
- `aws_security_group` (×2: one for ECS, one for MySQL)
- `aws_db_subnet_group`
- `aws_db_parameter_group`
- `aws_db_instance` (the MySQL RDS)
- `aws_cloudwatch_log_group`
- `aws_ecs_cluster`
- `aws_ecs_task_definition`
- `aws_ecs_service`
- `docker_image`
- `docker_registry_image`

### 3F: Apply — create all infrastructure

```powershell
terraform apply -var="db_password=YourSecurePass123!"
```

Terraform shows the plan again and asks: `Do you want to perform these actions?`

Type `yes` and press Enter.

**⏱ This takes 8-15 minutes.** Here's the rough timeline:
- Minutes 0-1: ECR repo, security groups, log group, ECS cluster created quickly
- Minutes 1-3: Docker image builds locally and pushes to ECR
- Minutes 3-12: **RDS instance provisioning** (this is the slow part — AWS is spinning up a MySQL server)
- Minutes 12-14: ECS task definition + service created, Fargate task starts launching

**📸 SCREENSHOT #1:** When `terraform apply` finishes, take a screenshot of the terminal showing the outputs:
```
Apply complete! Resources: 12 added, 0 changed, 0 destroyed.

Outputs:
ecs_cluster_name = "hw8-store-cluster"
ecs_service_name = "hw8-store"
rds_endpoint     = "hw8-store-mysql.xxxxxxxx.us-east-1.rds.amazonaws.com:3306"
rds_hostname     = "hw8-store-mysql.xxxxxxxx.us-east-1.rds.amazonaws.com"
rds_db_name      = "ecommerce"
```

**Write down the `rds_endpoint` and `ecs_cluster_name` — you'll need them.**

---

## STEP 4: Find Your ECS Task's Public IP

Your Go server is now running as a Fargate task, but you need its public IP to send requests.

### 4A: Get the task ARN

```powershell
aws ecs list-tasks --cluster hw8-store-cluster --service-name hw8-store --region us-east-1
```

**Output looks like:**
```json
{
    "taskArns": [
        "arn:aws:ecs:us-east-1:123456789:task/hw8-store-cluster/abc123def456..."
    ]
}
```

Copy the full task ARN (the entire string starting with `arn:aws:ecs:...`).

### 4B: Get the network interface (ENI) from the task

```powershell
aws ecs describe-tasks --cluster hw8-store-cluster --tasks "PASTE_TASK_ARN_HERE" --region us-east-1
```

In the big JSON output, look for this section:
```json
"attachments": [
    {
        "details": [
            { "name": "networkInterfaceId", "value": "eni-0abc123def456..." },
            ...
        ]
    }
]
```

Copy the `eni-...` value.

### 4C: Get the public IP from the ENI

```powershell
aws ec2 describe-network-interfaces --network-interface-ids "eni-PASTE_HERE" --region us-east-1
```

Look for:
```json
"Association": {
    "PublicIp": "3.95.XXX.XXX"
}
```

**That's your public IP.** Save it — I'll call it `<ECS_IP>` from now on.

### 4D: Alternative — use AWS Console (easier)

1. Open https://console.aws.amazon.com/ecs/
2. Make sure you're in **us-east-1** (top-right dropdown)
3. Click **Clusters** → **hw8-store-cluster**
4. Click the **Tasks** tab
5. Click the running task (the blue link)
6. Scroll to the **Configuration** section → **Network** subsection
7. You'll see **Public IP: 3.95.XXX.XXX**

**📸 SCREENSHOT #2:** Take a screenshot of the ECS task details page showing the public IP, task status "RUNNING", and the container status.

---

## STEP 5: Wait for the Server to Be Ready

The ECS task started, but it needs to connect to RDS. The Go server retries 30 times (2 seconds apart = up to 60 seconds).

### 5A: Check the logs to see connection progress

```powershell
aws logs tail /ecs/hw8-store --follow --region us-east-1
```

**What you'll see (in order):**
```
Waiting for MySQL (attempt 1/30): dial tcp ...
Waiting for MySQL (attempt 2/30): dial tcp ...
...
Connected to MySQL at hw8-store-mysql.xxx.us-east-1.rds.amazonaws.com:3306/ecommerce
Schema migration complete
Server running on :8080
```

Once you see `Server running on :8080`, the server is ready. Press `Ctrl+C` to stop tailing logs.

**If you see "could not connect to MySQL after 30 attempts":**
- The RDS instance might still be starting. Wait 2-3 minutes and check if ECS restarts the task automatically.
- Check that the RDS security group allows port 3306 from the ECS security group.

### 5B: Health check

```powershell
curl http://<ECS_IP>:8080/health
```

Replace `<ECS_IP>` with your actual IP from Step 4.

**Expected response:**
```json
{"status":"healthy"}
```

If you get `curl: (7) Failed to connect` — the task might still be starting. Wait 30 seconds and retry.

---

## STEP 6: Manual Smoke Test

Before running the performance test, verify each endpoint works.

### 6A: Create a shopping cart

```powershell
curl -X POST http://<ECS_IP>:8080/shopping-carts -H "Content-Type: application/json" -d "{\"customer_id\": 1}"
```

**Expected (201 Created):**
```json
{"shopping_cart_id": 1}
```

### 6B: Add an item to the cart

```powershell
curl -X POST http://<ECS_IP>:8080/shopping-carts/1/items -H "Content-Type: application/json" -d "{\"product_id\": 42, \"quantity\": 3}"
```

**Expected:** No body, just a 204 status. curl won't print anything — that's correct.

### 6C: Retrieve the cart with items

```powershell
curl http://<ECS_IP>:8080/shopping-carts/1
```

**Expected (200 OK):**
```json
{
  "cart_id": 1,
  "customer_id": 1,
  "items": [
    {
      "item_id": 1,
      "product_id": 42,
      "quantity": 3,
      "added_at": "2026-03-22T...",
      "updated_at": "2026-03-22T..."
    }
  ],
  "created_at": "2026-03-22T...",
  "updated_at": "2026-03-22T..."
}
```

### 6D: Test the upsert — add same product again

```powershell
curl -X POST http://<ECS_IP>:8080/shopping-carts/1/items -H "Content-Type: application/json" -d "{\"product_id\": 42, \"quantity\": 2}"
```

Then fetch again:

```powershell
curl http://<ECS_IP>:8080/shopping-carts/1
```

**Expected:** The quantity should now be **5** (3 + 2). This confirms the `ON DUPLICATE KEY UPDATE` upsert is working.

### 6E: Verify HW5 product endpoints still work

```powershell
curl -X POST http://<ECS_IP>:8080/products -H "Content-Type: application/json" -d "{\"product_id\":1,\"sku\":\"TEST-1\",\"manufacturer\":\"ACME\",\"category_id\":1,\"weight\":100,\"some_other_id\":1}"
```

**Expected (201 Created):** The product JSON echoed back.

```powershell
curl http://<ECS_IP>:8080/products/1
```

**Expected (200 OK):** The product you just created.

### 6F: Test error handling

```powershell
curl http://<ECS_IP>:8080/shopping-carts/99999
```

**Expected (404 Not Found):**
```json
{"error":"NOT_FOUND","message":"Shopping cart not found","details":"No cart with this ID exists"}
```

**📸 SCREENSHOT #3:** Take a screenshot showing the smoke test results — specifically the cart creation (201), item add (204), cart retrieval with items, and the upsert showing quantity = 5.

---

## STEP 7: Run the 150-Operation Performance Test

### 7A: Navigate to the loadTest directory

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\loadTest"
```

### 7B: Run the test

```powershell
python performance_test.py --host http://<ECS_IP>:8080
```

**What happens:** The script sends exactly 150 requests in 3 phases:
1. 50× `POST /shopping-carts` (create cart)
2. 50× `POST /shopping-carts/{id}/items` (add items)
3. 50× `GET /shopping-carts/{id}` (retrieve cart)

**What you'll see in the terminal:**
```
============================================================
  HW8 MySQL Performance Test
  Target: http://3.95.XXX.XXX:8080
  Time:   2026-03-22T15:30:00+00:00
============================================================

[Phase 1] Creating 50 shopping carts...
  Created 10/50  (last: 23.4ms, status: 201)
  Created 20/50  (last: 18.7ms, status: 201)
  ...
  ✓ 50 carts created successfully

[Phase 2] Adding items to 50 carts...
  Added 10/50   (last: 31.2ms, status: 204)
  ...
  ✓ Item additions complete

[Phase 3] Retrieving 50 carts...
  Retrieved 10/50 (last: 15.8ms, status: 200)
  ...
  ✓ Retrievals complete

============================================================
  RESULTS SUMMARY
============================================================

  create_cart:
    Success: 50/50
    Avg:     22.3ms
    Min:     12.1ms
    Max:     45.6ms
    P95:     38.2ms

  add_items:
    Success: 50/50
    Avg:     28.7ms
    ...

  get_cart:
    Success: 50/50
    Avg:     18.4ms
    ...

  TOTAL: 150/150 successful
============================================================

Results saved to mysql_test_results.json

Total test duration: 12.3s
✓  Completed within 5-minute window (12s / 300s)
```

**📸 SCREENSHOT #4:** Take a screenshot of the complete results summary showing all three operation types, their average/min/max/P95 times, and the total success count.

### 7C: Verify the output file exists

```powershell
dir mysql_test_results.json
```

This file is at `C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\loadTest\mysql_test_results.json`.

**⚠️ CRITICAL: Do NOT delete this file. You need it for the Week 6c comparison analysis.**

---

## STEP 8: Run Locust Load Test (for deeper analysis)

### 8A: Install Locust (if not already installed)

```powershell
pip install locust
```

### 8B: Start the Locust web UI

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\loadTest"
locust -f locust_shopping_cart.py --host http://<ECS_IP>:8080
```

### 8C: Open the Locust UI

Open your browser and go to: **http://localhost:8089**

### 8D: Run Test 1 — Light load

Enter these settings:
- **Number of users:** 10
- **Spawn rate:** 2
- Click **Start swarming**

Let it run for **2 minutes**. Watch the:
- **Requests/s** column
- **Average (ms)** and **95%ile** columns
- **Failure** count (should be 0)

**📸 SCREENSHOT #5:** After 2 minutes at 10 users, take a screenshot of the Locust statistics table showing all three endpoints, their request counts, average/median/P95 response times, and 0 failures.

### 8E: Run Test 2 — Medium load

Click **Edit** (top of Locust UI), change to:
- **Number of users:** 50
- **Spawn rate:** 5
- Click **Start swarming**

Let it run for **2 minutes**.

**📸 SCREENSHOT #6:** After 2 minutes at 50 users, take a screenshot of the Locust statistics table. Note how the response times change compared to 10 users.

### 8F: Run Test 3 — Heavy load (stress test)

Click **Edit**, change to:
- **Number of users:** 100
- **Spawn rate:** 10
- Click **Start swarming**

Let it run for **2 minutes**.

**📸 SCREENSHOT #7:** After 2 minutes at 100 users, screenshot the statistics table AND the **Charts** tab (click "Charts" at the top). The charts show:
- Total Requests per Second over time
- Response Times over time
- Number of Users over time

### 8G: Stop Locust

Click **Stop** in the Locust UI. Press `Ctrl+C` in the terminal where Locust is running.

---

## STEP 9: CloudWatch Monitoring Screenshots

While the Locust tests were running (or immediately after), go to the AWS Console to capture metrics.

### 9A: RDS Metrics

1. Go to: **https://console.aws.amazon.com/rds/**
2. Make sure you're in **us-east-1**
3. Click **Databases** in the left sidebar
4. Click on **hw8-store-mysql**
5. Click the **Monitoring** tab

**📸 SCREENSHOT #8:** Take a screenshot showing these RDS metrics (select the monitoring period that covers your test window):
- **CPUUtilization** — how hard the database worked
- **DatabaseConnections** — should show connections from the pool (up to 20)
- **ReadLatency** and **WriteLatency** — I/O performance
- **FreeableMemory** — memory headroom

To see all of these at once, you may need to scroll. You can also click on individual metrics to zoom in.

**📸 SCREENSHOT #9:** Click on **DatabaseConnections** specifically and take a close-up screenshot. This shows how the connection pool behaves — you should see it ramp up during Locust tests and stay steady (not spike to 150).

### 9B: ECS Metrics

1. Go to: **https://console.aws.amazon.com/ecs/**
2. Click **Clusters** → **hw8-store-cluster**
3. Click the **Metrics** tab

**📸 SCREENSHOT #10:** Take a screenshot of the ECS cluster metrics:
- **CPUUtilization** — how hard the Go server worked
- **MemoryUtilization** — memory usage of the Fargate task

### 9C: CloudWatch Logs (optional but valuable)

1. Go to: **https://console.aws.amazon.com/cloudwatch/**
2. Click **Log groups** in the left sidebar
3. Click **/ecs/hw8-store**
4. Click the latest log stream

**📸 SCREENSHOT #11:** Take a screenshot of the log stream showing:
- The MySQL connection retry messages
- "Connected to MySQL" message
- "Schema migration complete" message
- "Server running on :8080" message

This proves the server connected to RDS successfully and ran the migration.

---

## STEP 10: Tear Down All Resources

### 10A: Destroy everything

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\terraform-config\terraform"
terraform destroy -auto-approve -var="db_password=YourSecurePass123!"
```

**⚠️ Use the SAME `db_password` you used during `terraform apply`.** If you don't remember it, it's okay — Terraform reads from the state file, but the variable still needs to be provided.

**What this destroys (takes 5-10 minutes):**
- ECS service and tasks (Go server stops)
- ECS cluster
- Task definition
- RDS instance (3-5 minutes to delete)
- Security groups
- DB subnet group + parameter group
- CloudWatch log group
- ECR repository
- Docker registry image

**📸 SCREENSHOT #12:** Take a screenshot of the terminal showing:
```
Destroy complete! Resources: 12 destroyed.
```

### 10B: Verify nothing is left running (important for AWS billing)

```powershell
aws rds describe-db-instances --region us-east-1
aws ecs list-clusters --region us-east-1
```

Both should return empty lists (or not include any `hw8-store` resources).

---

## Summary of All Screenshots

| # | What | Where | Why |
|---|------|-------|-----|
| 1 | `terraform apply` output | Terminal | Proves infrastructure deployed successfully |
| 2 | ECS task details with Public IP | AWS Console (ECS) | Shows running task + network config |
| 3 | Smoke test curl results | Terminal | Proves all endpoints work correctly |
| 4 | 150-operation test summary | Terminal | Performance test results with timing data |
| 5 | Locust @ 10 users | Browser (localhost:8089) | Baseline performance under light load |
| 6 | Locust @ 50 users | Browser (localhost:8089) | Performance under medium load |
| 7 | Locust @ 100 users + charts | Browser (localhost:8089) | Stress test + response time trends |
| 8 | RDS monitoring dashboard | AWS Console (RDS) | Database CPU, connections, latency, memory |
| 9 | RDS DatabaseConnections close-up | AWS Console (RDS) | Connection pool behavior |
| 10 | ECS cluster metrics | AWS Console (ECS) | Server CPU + memory utilization |
| 11 | CloudWatch logs | AWS Console (CloudWatch) | Server startup + MySQL connection logs |
| 12 | `terraform destroy` output | Terminal | Proves cleanup completed |

---

## Key Files to Submit

1. **All Terraform code:** `Hw8/terraform-config/terraform/` (all .tf files + modules/)
2. **Go source code:** `Hw8/terraform-config/src/` (main.go, db/db.go, db/cart.go, Dockerfile, go.mod, go.sum)
3. **Performance test results:** `Hw8/loadTest/mysql_test_results.json` ← CRITICAL for Week 6c
4. **Test scripts:** `Hw8/loadTest/performance_test.py` + `locust_shopping_cart.py`
5. **Implementation notes:** `Hw8/docs/implementation-notes.md`
6. **Screenshots:** Save all 12 screenshots to an `Hw8/images/` folder
7. **This guide (optional):** `Hw8/docs/execution-guide.md`
