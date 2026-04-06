# Mock Interview Prep: HW7 — Sync vs Async vs Serverless
## Nithish Kadam Ganesh

---

## THE 30-SECOND ELEVATOR PITCH

*"If they ask 'walk me through your project' — say this:"*

We built an e-commerce order processing system on AWS and tested three architectures under flash-sale load. The sync path — where customers wait for 3-second payment verification — broke at 68% failure rate under 20 concurrent users. We decoupled acceptance from processing using SNS and SQS, which handled 124x more orders at 36ms response time with zero failures. Then we replaced the ECS workers with Lambda, which eliminated all operational overhead and costs $0 under the free tier, with only a 70ms cold start penalty — 2.3% on a 3-second operation. The key insight is that serverless isn't about performance, it's about eliminating operational complexity.

---

## LIKELY QUESTIONS AND STRONG ANSWERS

### Q1: "Why can't you just use time.Sleep to simulate the bottleneck?"

This is probably the most technically interesting question they'll ask. Nail it.

**Answer:** "In Go, when a goroutine calls `time.Sleep`, the runtime parks that goroutine — but the OS thread is NOT blocked. Go's scheduler can handle thousands of sleeping goroutines on just a few OS threads. So if I used `time.Sleep(3s)` alone, the HTTP server would happily accept unlimited concurrent requests — each one would spawn a goroutine, sleep independently, and return. There'd be no bottleneck at all.

Instead, I used a **buffered channel as a semaphore** — this is actually straight from Effective Go. The channel has capacity 1, so only one goroutine can hold the slot at a time. When the channel is full, the send operation genuinely blocks the calling goroutine until a slot opens. This creates real backpressure: if the payment processor is busy, new HTTP requests block on the channel send, which blocks the handler goroutine, which makes the client wait. That's the realistic simulation of a slow external dependency."

**If they follow up: "What's the difference between blocking a goroutine and blocking a thread?"**

"Go's M:N scheduler multiplexes many goroutines (G) onto a smaller number of OS threads (M). When a goroutine sleeps or does I/O, the scheduler parks it and runs another goroutine on the same thread — so the thread stays productive. But when a goroutine blocks on a channel send to a full buffered channel, it's parked until another goroutine receives from the channel. The key difference is that sleep doesn't create contention — the scheduler just comes back later. Channel blocking creates genuine contention for a shared resource."

---

### Q2: "Walk me through the async architecture."

**Answer:** "The flow is: Client hits the ALB, which routes to the order-receiver ECS service. The `/orders/async` endpoint creates an Order struct, serializes it to JSON, publishes it to an SNS topic, and immediately returns 202 Accepted — the whole thing takes about 32ms.

SNS then delivers that message to an SQS queue. A separate ECS service — the order-processor — runs a polling loop: it calls `ReceiveMessage` with long polling (waits up to 20 seconds for messages, returns up to 10 at a time), spawns a goroutine per message, each goroutine acquires a semaphore slot, processes the payment for 3 seconds, deletes the message from SQS, and loops back.

The critical decoupling is: the customer's HTTP response time is now independent of payment processing time. The API just needs to publish to SNS, which takes milliseconds."

**If they ask: "Why SNS in front of SQS? Why not publish directly to SQS?"**

"SNS acts as a fan-out layer. In Part III, when we added Lambda, we just subscribed Lambda to the same SNS topic — without changing a single line of the order-receiver code. The receiver publishes once; SNS delivers to all subscribers. If tomorrow we want to add an analytics queue or an email notification service, we just add another subscription. That's the power of the pub/sub pattern."

---

### Q3: "What happened during the sync flash sale test?"

**Answer:** "Catastrophic failure. With 20 concurrent users and a capacity-1 semaphore, only one payment could process at a time — 0.33 orders per second. The other 19 users' requests piled up behind the semaphore. Most hit the 30-second timeout. From my CSV data: 28 total requests in 60 seconds, 19 failed (67.86% failure rate), 25.7 second average response time, and every percentile from P50 to P100 was at 30,000ms — the timeout ceiling. Throughput was 0.50 req/s.

Compare that to async: 3,460 requests, zero failures, 36ms average, 58.15 req/s. That's 124x more orders handled."

---

### Q4: "What's the queue problem you discovered?"

**Answer:** "The async system has 100% acceptance — every order gets a 202 instantly. But with 1 worker processing at 0.33 orders/sec and 58 orders/sec arriving, the queue grows at about 57.8 messages per second. After a 60-second flash sale, there are ~3,460 messages in the queue. At 0.33/sec drain rate, that's 175 minutes — almost 3 hours — to clear. Customers got instant acceptance but would wait hours for their order confirmation.

The solution is worker scaling. I tested 1, 5, 20, and 100 goroutines. The API-side metrics were identical across all tests — confirming the decoupling works. The difference showed up in CloudWatch: messages deleted per test went from ~20 (1 worker) to ~2,000 (100 workers). The minimum to prevent buildup at 58 req/s is 58 x 3 = 174 concurrent workers."

---

### Q5: "What's interesting about the resource utilization?"

**Answer:** "Even at 100 concurrent goroutines, the processor used only 20% of CPU (51 out of 256 units) and 30% of memory (154 of 512 MB). That's because the goroutines spend their time in `time.Sleep` — sleeping goroutines consume zero CPU and only about 8KB of stack memory each. The bottleneck was never hardware — it was purely the semaphore-gated 3-second processing time.

This is actually a key insight about Go's concurrency model. In a real system with actual network I/O to a payment gateway, CPU would still be low (it's I/O bound, not CPU bound), but memory would grow more because of connection state, TLS buffers, and response parsing."

---

### Q6: "Explain the SQS configuration choices."

**Answer:** "Three key parameters:

**Visibility timeout (30 seconds):** When a worker receives a message, SQS hides it from other workers for 30 seconds. If the worker finishes and deletes it — great. If the worker crashes, the message reappears after 30 seconds for another worker to pick up. This must be longer than processing time (3 seconds) to avoid duplicate processing during normal operation. This gives us at-least-once delivery, which means processing needs to be idempotent.

**Long polling (20 seconds):** Without this, `ReceiveMessage` returns immediately even if the queue is empty — burning API calls and adding latency. With long polling, SQS holds the connection open for up to 20 seconds waiting for messages. This reduces costs and delivers messages faster.

**Message retention (4 days):** If all workers go down, messages survive for 4 days. This gives the ops team time to fix issues without losing orders."

---

### Q7: "Tell me about Lambda and cold starts."

**Answer:** "For Part III, I deployed a Lambda function subscribed directly to the SNS topic — no SQS in between. Same 3-second payment simulation. I sent 10 test orders manually (not Locust — the assignment warned it could deactivate your account).

The first invocation had an Init Duration of 70.11ms — that's the cold start where Lambda provisions the runtime environment. The second invocation, 4 seconds later, had no Init Duration — it reused the warm instance. The overhead is 70ms on a 3-second operation, which is 2.3%. For this workload, it's completely irrelevant.

Cold starts happen on the first invocation and after roughly 5 minutes of idle time. Go is actually one of the fastest runtimes for cold starts — Java or .NET can be 500ms to several seconds."

---

### Q8: "Should the startup switch to Lambda?"

**Answer:** "Yes, at startup volumes. Lambda is free until 267,000 orders per month under the free tier, while ECS costs $17/month always running regardless of traffic. But cost isn't the real argument — it's operational overhead. With ECS+SQS, you have 3am queue depth alerts, manual worker scaling, visibility timeout tuning, ECS health monitoring. Lambda eliminates all of that.

The trade-off is losing SQS message persistence and retry guarantees. SNS retries Lambda only twice before discarding. But for this use case — where the API already returned 202 and the order is presumably stored in a database — that's acceptable.

The migration path is clean because SNS is the integration point. Today both SQS and Lambda subscribe to the same topic. You can run them in parallel during migration, and if Lambda doesn't work out at scale, the SQS+ECS architecture is still there."

---

### Q9: "How did your results compare to your teammates'?"

**Answer:** "The interesting thing is we all used different semaphore capacities. I used capacity 1 — the strictest bottleneck. Santrupti used 5, Jatin used 15. So our sync throughput ceilings were 0.33, 1.67, and 5 req/s respectively. My sync test showed the most dramatic failure — 68% failure rate — while Jatin's 15-slot setup had zero failures but still capped at 5 req/s.

But here's the key finding: when we switched to async, all three of us saw 0% failure rates. Santrupti hit 180 req/s, Jatin and I both saw ~56-58 req/s. The absolute numbers differed, but the conclusion was the same: async decoupling eliminates customer-facing failures regardless of the backend bottleneck.

For Lambda, our cold starts ranged from 46ms (Santrupti) to 119ms (Jatin) to 70ms (mine). All negligible on a 3-second operation. And all three of us independently recommended Lambda for low-volume startups."

---

### Q10: "What would you change or add if you had more time?"

**Answer:** "A few things:

1. **Dead-letter queue.** I'd add an SQS DLQ for messages that fail processing after N retries. Right now a poison message could loop forever.

2. **Auto-scaling.** Instead of manually changing WORKER_COUNT via Terraform, I'd set up a CloudWatch alarm on `ApproximateNumberOfMessagesVisible` that triggers ECS service auto-scaling — adding more tasks when the queue is deep.

3. **Idempotency.** The at-least-once delivery from SQS means an order could be processed twice if the worker crashes after payment but before deleting the message. I'd add an idempotency key (order ID) checked against a database before processing.

4. **Observability.** I'd add distributed tracing (X-Ray) so you can follow an order from the API through SNS, SQS, and the processor. Right now the correlation is only through log messages."

---

## RAPID-FIRE TECHNICAL TERMS TO KNOW

| Term | What to say |
|------|-------------|
| **Buffered channel** | "Fixed-capacity queue in Go. Sends block when full, receives block when empty. I used it as a semaphore to limit concurrent payments." |
| **Semaphore** | "A concurrency primitive that limits the number of goroutines accessing a resource simultaneously. My buffered channel with capacity N acts as a counting semaphore." |
| **SNS** | "Pub/sub messaging service. Publisher sends once, SNS delivers to all subscribers. I used it as the fan-out layer between the API and both SQS and Lambda." |
| **SQS** | "Managed message queue with at-least-once delivery, visibility timeouts, and message retention. Decouples producers from consumers." |
| **Long polling** | "SQS holds the connection open up to 20 seconds waiting for messages, instead of returning empty immediately. Reduces API calls and delivers messages faster." |
| **Visibility timeout** | "When a worker receives a message, SQS hides it for 30 seconds. If the worker doesn't delete it in time, it reappears for another worker. Provides at-least-once delivery." |
| **202 Accepted** | "HTTP status code meaning 'I've received your request and it will be processed asynchronously.' Different from 200 OK which means 'done right now.'" |
| **Cold start** | "First Lambda invocation provisions the runtime. My Go function had 70ms init. Subsequent invocations reuse the warm container." |
| **Fargate** | "Serverless compute for containers. You define CPU/memory, AWS manages the underlying EC2 instances. I used 256 CPU units and 512MB memory." |
| **Backpressure** | "When a system slows down upstream when downstream can't keep up. My buffered channel creates backpressure — if payment processing is full, HTTP handlers block." |
| **Fan-out** | "One message delivered to many consumers. SNS fans out to both SQS (for ECS workers) and Lambda simultaneously." |
| **Eventual consistency** | "The customer gets a 202 instantly, but the order isn't actually processed yet. They'll eventually get confirmation. Acceptable for slow operations like payment." |
| **Idempotency** | "Processing the same message twice produces the same result. Important because SQS provides at-least-once delivery, meaning duplicates are possible." |

---

## YOUR KEY NUMBERS (MEMORIZE THESE)

| What | Number |
|------|--------|
| Sync flash: failure rate | 67.86% |
| Sync flash: avg response | 25,724 ms |
| Sync flash: throughput | 0.50 req/s |
| Async flash: failure rate | 0% |
| Async flash: avg response | 36 ms |
| Async flash: throughput | 58.15 req/s |
| Improvement multiplier | 124x more orders |
| Queue growth (1 worker) | ~58 msg/sec |
| Time to drain (1 worker) | ~175 minutes |
| Time to drain (100 workers) | ~1.7 minutes |
| Min workers to match demand | 174 |
| Lambda cold start | 70.11 ms (2.3% overhead) |
| Lambda cost at 10K orders/month | $0 (free tier) |
| Lambda free until | 267K orders/month |
| ECS cost | $17/month always |
| Processor CPU at 100 workers | 20.1% of 256 units |
| Processor memory at 100 workers | 30.0% of 512 MB |

---

## TRICKY FOLLOW-UPS THEY MIGHT ASK

**"What if a message fails processing?"**
"With the current setup, the message stays invisible for the 30-second visibility timeout, then reappears in the queue for another worker to pick up. In production, I'd add a dead-letter queue after N failed attempts."

**"What about message ordering?"**
"SQS standard queues don't guarantee ordering. For this use case that's fine — order A and order B can process in any sequence. If ordering mattered, we'd use SQS FIFO queues, which guarantee first-in-first-out within a message group."

**"Why not just increase the ECS task CPU and memory?"**
"That wouldn't help. The bottleneck isn't CPU or memory — my Container Insights showed only 20% CPU and 30% memory at 100 workers. The bottleneck is the 3-second payment processing time. The only way to increase throughput is more concurrent workers."

**"Could you auto-scale the workers?"**
"Yes. I'd create a CloudWatch alarm on `ApproximateNumberOfMessagesVisible` that triggers ECS auto-scaling — increasing the number of processor tasks when queue depth exceeds a threshold. Each new task would bring its own set of WORKER_COUNT goroutines."

**"What's the difference between scaling goroutines within a task vs scaling tasks?"**
"Scaling goroutines within a single task is faster (just a config change, same container) but limited by the task's CPU/memory allocation. Scaling tasks horizontally adds more containers, each with their own CPU/memory budget, but takes longer (ECS needs to provision new Fargate instances). In practice you'd do both: goroutines for fine-grained scaling within a task, task count for coarse scaling when you need more total resources."

**"Why did Santrupti get 180 req/s while you got 58 req/s?"**
"Likely differences in Locust wait-time configuration. The assignment says 100-500ms between requests, but if Santrupti used a shorter wait time or ran Locust with the web UI (which has slightly different timing characteristics), the effective request rate would be higher. Our configurations were independently developed, which is why the absolute numbers differ but the conclusions align."

**"What happens if SNS can't deliver to Lambda?"**
"SNS retries Lambda invocations twice. If all three attempts fail, the message is discarded — that's the trade-off vs SQS where messages persist for up to 4 days. In production, you could configure an SNS dead-letter queue to capture failed deliveries."
