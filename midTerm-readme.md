# Topics I Enjoyed Learning in CS 6650

This reflection draws from class slides (Days 1–6), assignments (HW1–HW6), and readings (Lamport, Parnas, Mixu Ch. 1–2).

---

## 1) From One Machine to Many — The Four Forces

Distributed systems feel overwhelming until you notice the same four forces at every layer: **coordination** (getting parties to agree), **contention** (fighting over shared resources), **partial failure** (some parts break while others run), and **observability** (making the invisible visible). The course kept zooming out — goroutines → containers → services → clusters — but these forces never changed, only the scale did.

```
  Single Process:   [Client] → [Handler] → [In-Memory State]
  Distributed:      [Client] → [ALB] → [Task A/B/C] → [Shared DB]
                                             │
                                     [CloudWatch + Auto-Scaling]
```

The shift: state moves out of local memory, a load balancer replaces direct calls, health checks replace trust, and metrics become your eyes.

## 2) Concurrency — Where Bugs Hide in Plain Sight

HW3 turned concurrency from abstract to visceral. A shared counter gave nondeterministic results across goroutines — not crashing, just *silently wrong*. Go's `-race` detector exposed bugs that could run 99 times clean and corrupt on the 100th.

| Approach | Correctness | Throughput | Best When |
| :--- | :---: | :--- | :--- |
| Plain `map` | ❌ | Fastest (unsafe) | Never in concurrent code |
| `sync.Mutex` | ✅ | Bottlenecks under contention | Write-heavy, simple safety |
| `sync.RWMutex` | ✅ | Better with many readers | Read-heavy workloads |
| `sync.Map` | ✅ | Pattern-dependent | Stable keys, concurrent reads |

**The lesson:** performance numbers are meaningless without knowing your read/write ratio and contention level.

## 3) Coordination — The Cost of Agreement

The leap from single-machine ACID to multi-party correctness was eye-opening. Two-Phase Commit made me see the difference between making data **durable** (safely on disk) and **globally agreed** (everyone commits or nobody does). The elegant failure: if the coordinator dies after collecting "yes" votes but before sending "commit," every participant is stuck. Strong guarantees always trade away simplicity and introduce new ways to fail.

## 4) APIs to Load Balancers — Where Code Meets Infrastructure

Writing REST handlers in Go felt familiar. Moving them off `localhost` changed every question — from *"is my function correct?"* to *"which replica handled this, and what does its p99 look like?"* The load balancer crystallized a beautiful abstraction: **requests are a stream, instances are interchangeable, health checks decide who's in the game.**

## 5) Reproducibility — Terraform, Docker, and the End of "I Remember the Steps"

| Before | After |
| :--- | :--- |
| 12 manual console clicks | `terraform apply` |
| "Works on my machine" | Docker image runs identically everywhere |
| "I think I deleted everything" | `terraform destroy` — verified |
| SSH in to deploy | Push to ECR → ECS rolls out automatically |

The **ECR → ECS → Fargate** chain cleanly separated image storage, orchestration, and runtime. But the most valuable habit wasn't a tool — it was **lifecycle thinking**: always destroy resources, always track spend, always treat infrastructure as reviewable code.

## 6) Tail Latency — Why Averages Lie

Load testing with Locust taught me to distrust averages. A median of 12ms can hide a p95 of 310ms — that tail is what users feel as *"sometimes it's just slow."* HW6's bottleneck diagnosis was clean: CPU pinned at 95%+ while memory sat at ~35%. When every request scans 100 products, there's no algorithmic shortcut. The question shifts from *"how do I optimize?"* to *"how do I scale?"*

## 7) Auto-Scaling — Watching Resilience Come Alive

The ALB + auto-scaling setup turned elasticity into something I could *watch*. Load rises → CPU crosses 70% → CloudWatch fires → ECS spins up tasks → ALB registers them → latency stabilizes. The most memorable moment: **killing a task mid-load-test** — instead of a catastrophe, the system shrugged. Traffic rerouted, replicas absorbed the load, and a replacement came online. That single experiment taught me more about resilience than any lecture.

## 8) Readings That Changed My Default Assumptions

Before this course, I assumed clocks were reliable, modules were just folders, and failures were rare. The readings dismantled all three. **Lamport** showed that in a distributed world there is no universal "now" — you reason about causality through happens-before, not timestamps. **Parnas** reframed modularity as information-hiding, not code organization — exactly why microservices work when done right. **Mixu** (Ch. 1–2) laid out the ground rules: networks are unreliable, nodes fail independently, and coordination always has a cost.

---

> The topics I enjoyed most forced me to **collect evidence and defend conclusions** — concurrency experiments, load testing, tail latency analysis, and scaling decisions. That evidence-first mindset is the real throughline of CS 6650.