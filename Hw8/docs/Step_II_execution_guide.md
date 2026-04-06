# HW8 Step II — DynamoDB: Complete Execution Guide

> **Starting point:** You have the Step I MySQL infrastructure already destroyed.
> Your project lives at: `C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8`

---

## STEP 1: Generate Updated go.sum

The go.mod now includes the AWS SDK v2 for DynamoDB. You need to regenerate go.sum.

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\terraform-config\src"
go mod tidy
```

This downloads the AWS SDK v2 packages (`aws-sdk-go-v2`, `service/dynamodb`, `feature/dynamodb/attributevalue`, etc.) and updates go.sum.

**Verify it compiles:**
```powershell
go build -o server.exe .
```
Should compile cleanly (you can't run it locally without AWS credentials, but it should build).

---

## STEP 2: Ensure AWS Credentials Are Active

```powershell
aws sts get-caller-identity
```

If expired, re-run `aws configure` with your credentials and session token.

---

## STEP 3: Initialize and Deploy Terraform

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\terraform-config\terraform"
terraform init
```

**What's new:** Terraform discovers the new `dynamodb` module alongside the existing ones.

**Preview:**
```powershell
terraform plan -var="db_password=oxymoroN1"
```

You should see ~13 resources — the same 12 from Step I plus 1 new DynamoDB table (`aws_dynamodb_table.shopping_carts`).

**Deploy:**
```powershell
terraform apply -var="db_password=oxymoroN1"
```

Type `yes`. DynamoDB tables create in seconds (unlike RDS which takes 5-10 min). The RDS instance is still the bottleneck.

**📸 SCREENSHOT #1:** When `terraform apply` finishes, screenshot the outputs — you should now see `dynamodb_table_name = "hw8-store-dynamo-carts"` alongside the RDS outputs.

---

## STEP 4: Find Your ECS Task's Public IP

Same as Step I:

```powershell
aws ecs list-tasks --cluster hw8-store-cluster --service-name hw8-store --region us-east-1
```

Then describe the task to get the ENI, then the public IP. Or use the AWS Console: **ECS → Clusters → hw8-store-cluster → Tasks → click task → Networking → Public IP**.

Save this IP as `<ECS_IP>`.

---

## STEP 5: Verify Both Backends Are Running

```powershell
curl.exe http://<ECS_IP>:8080/health
```

**Expected:**
```json
{"dynamodb":"healthy","mysql":"healthy"}
```

If DynamoDB shows an error, check CloudWatch logs:
```powershell
aws logs tail /ecs/hw8-store --follow --region us-east-1
```

Look for `Connected to DynamoDB table: hw8-store-dynamo-carts`.

**📸 SCREENSHOT #2:** Screenshot the health check showing both MySQL and DynamoDB healthy.

---

## STEP 6: Smoke Test DynamoDB Endpoints

All DynamoDB endpoints have `/dynamo/` prefix. Test in Postman or curl:

**Create a cart (DynamoDB):**

- **Method:** POST
- **URL:** `http://<ECS_IP>:8080/dynamo/shopping-carts`
- **Body (raw JSON):**
```json
{
  "customer_id": 1
}
```
- **Expected:** 201 with `{"shopping_cart_id": 1}`

**Add item (DynamoDB):**

- **Method:** POST
- **URL:** `http://<ECS_IP>:8080/dynamo/shopping-carts/1/items`
- **Body (raw JSON):**
```json
{
  "product_id": 42,
  "quantity": 3
}
```
- **Expected:** 204 No Content

**Get cart (DynamoDB):**

- **Method:** GET
- **URL:** `http://<ECS_IP>:8080/dynamo/shopping-carts/1`
- **Expected:** 200 with the cart including items

**Upsert test — add same product again:**

- **Method:** POST
- **URL:** `http://<ECS_IP>:8080/dynamo/shopping-carts/1/items`
- **Body:** `{"product_id": 42, "quantity": 2}`
- Then GET again — quantity should be **5** (3 + 2)

**Error handling:**

- **Method:** GET
- **URL:** `http://<ECS_IP>:8080/dynamo/shopping-carts/99999`
- **Expected:** 404

**📸 SCREENSHOT #3:** Screenshot showing DynamoDB cart creation, item add, retrieval with upsert, and error handling.

---

## STEP 7: Run the 150-Operation Performance Test (DynamoDB)

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\loadTest"
python dynamodb_performance_test.py --host http://<ECS_IP>:8080
```

This runs the IDENTICAL test as Step I (50 create, 50 add, 50 get) but against the `/dynamo/` endpoints.

**📸 SCREENSHOT #4:** Screenshot the full results summary.

**⚠️ CRITICAL:** This produces `dynamodb_test_results.json` — needed for Step III comparison alongside `mysql_test_results.json`.

---

## STEP 8: Run the Eventual Consistency Test

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\loadTest"
python consistency_test.py --host http://<ECS_IP>:8080
```

**What this tests:**
1. **Read-After-Write:** Creates 20 carts and immediately reads them — measures how often the read finds the cart
2. **Add-Then-Read:** Adds 20 items and immediately reads the cart — checks if items appear instantly
3. **Rapid Updates:** Fires 10 rapid additions to the same product — checks if any quantity updates are lost

**📸 SCREENSHOT #5:** Screenshot the full consistency test output showing miss rates for all three tests.

---

## STEP 9: Run Locust Load Test (DynamoDB)

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\loadTest"
locust -f locust_dynamo_cart.py --host http://<ECS_IP>:8080
```

Open **http://localhost:8089** in your browser.

**Test 1 — Light load (10 users, spawn rate 2):** Run for 2 minutes.

**📸 SCREENSHOT #6:** Statistics table + Charts tab at 10 users.

**Test 2 — Medium load (50 users, spawn rate 5):** Run for 2 minutes.

**📸 SCREENSHOT #7:** Statistics + Charts at 50 users.

**Test 3 — Heavy load (100 users, spawn rate 10):** Run for 2 minutes.

**📸 SCREENSHOT #8:** Statistics + Charts at 100 users.

---

## STEP 10: CloudWatch Monitoring — DynamoDB Metrics

**DynamoDB metrics** (AWS Console → DynamoDB → Tables → hw8-store-dynamo-carts → Metrics tab):

**📸 SCREENSHOT #9:** Screenshot showing:
- **ConsumedReadCapacityUnits** and **ConsumedWriteCapacityUnits** — shows actual throughput
- **SuccessfulRequestLatency** — should show sub-10ms for GetItem
- **ThrottledRequests** — should be 0 (on-demand billing has no throttling under normal load)

**ECS metrics** (same as Step I):

**📸 SCREENSHOT #10:** CPU and Memory utilization during DynamoDB load tests.

---

## STEP 11: Tear Down

```powershell
cd "C:\Users\nithi\OneDrive\Desktop\DISTRIBUTED SYSTEMS\Hw8\terraform-config\terraform"
terraform destroy -auto-approve -var="db_password=oxymoroN1"
```

DynamoDB tables delete instantly (unlike RDS which takes minutes).

**📸 SCREENSHOT #11:** Destroy complete output.

---

## Summary of All Step II Screenshots

| # | What | Where |
|---|------|-------|
| 1 | `terraform apply` with DynamoDB table output | Terminal |
| 2 | Health check showing both MySQL + DynamoDB healthy | Terminal/Postman |
| 3 | DynamoDB smoke test (create, add, get, upsert, 404) | Postman |
| 4 | 150-operation DynamoDB performance test summary | Terminal |
| 5 | Consistency test — 3 tests with miss rates | Terminal |
| 6 | Locust DynamoDB @ 10 users | Browser |
| 7 | Locust DynamoDB @ 50 users | Browser |
| 8 | Locust DynamoDB @ 100 users | Browser |
| 9 | DynamoDB CloudWatch metrics (capacity, latency) | AWS Console |
| 10 | ECS CPU/Memory during DynamoDB tests | AWS Console |
| 11 | `terraform destroy` output | Terminal |

---

## Key Files for Step III Comparison

After running both tests, you should have:
- `loadTest/mysql_test_results.json` (from Step I)
- `loadTest/dynamodb_test_results.json` (from Step II)
- `loadTest/consistency_test_results.json` (DynamoDB-specific)

---

## API Endpoints Summary (Both Backends)

| Method | MySQL Path | DynamoDB Path | Description |
|--------|-----------|---------------|-------------|
| POST | `/shopping-carts` | `/dynamo/shopping-carts` | Create cart |
| GET | `/shopping-carts/{id}` | `/dynamo/shopping-carts/{id}` | Get cart + items |
| POST | `/shopping-carts/{id}/items` | `/dynamo/shopping-carts/{id}/items` | Add/update item |
| GET | `/health` | `/health` | Health (shows both) |
