# Mutex and RWMutex Experiment Report

## Synchronizing Concurrent Access to a Shared Map in Go

## Experiment Overview

This experiment explores how mutual exclusion mechanisms ensure correctness when multiple goroutines access a shared map concurrently. Two synchronization strategies were evaluated:

1. `sync.Mutex`
2. `sync.RWMutex`

For both experiments, 50 goroutines were spawned, and each goroutine performed 1,000 write operations to a shared `map[int]int` using unique keys. The program measured the final map size and the total execution time.

The expected map size in all cases was 50,000.

---

## Mutex Experiment

### Setup

The shared map was wrapped inside a struct containing a `sync.Mutex`. Before every write operation, the mutex was locked and unlocked immediately after the write. This ensured that only one goroutine could modify the map at any given time.

### Results

The experiment was run three times.

- The program completed successfully in all runs.
- The map size was consistently 50,000.
- The execution times observed were:

| Run | Execution Time |
|-----|----------------|
| 1   | 10.489 ms      |
| 2   | 11.1629 ms     |
| 3   | 11.0021 ms     |

**Mean execution time: ~10.88 ms**

![Terminal output showing successful execution of mutex_map.go](./images/s5.png)

### Interpretation

The mutex guarantees correctness by enforcing strict mutual exclusion. This prevents concurrent structural modification of the map and preserves its internal consistency. However, this safety comes at the cost of serialized access, reducing parallelism and increasing overall execution time compared to an unsafe implementation.

---

## RWMutex Experiment

### Setup

The mutex was replaced with a `sync.RWMutex`. Since all operations in this experiment were writes, each goroutine acquired an exclusive write lock (`Lock` / `Unlock`) for every map update.

### Results

The experiment was again run three times.

- The map size was consistently 50,000.
- The execution times observed were:

| Run | Execution Time |
|-----|----------------|
| 1   | 11.4749 ms     |
| 2   | 11.6455 ms     |
| 3   | 13.9491 ms     |

**Mean execution time: ~12.36 ms**

![Terminal output showing successful execution of rwmutex_map.go](./images/s6.png)

### Interpretation

The RWMutex did not improve performance in this workload. Because all operations were writes, the RWMutex still required exclusive access, providing no additional concurrency. The extra internal bookkeeping associated with RWMutex resulted in slightly higher execution times compared to the regular mutex.

---

## Lessons Learned

- Synchronization is required to safely share data structures across concurrent goroutines.
- A mutex guarantees correctness by serializing access, but limits concurrency.
- RWMutex only provides performance benefits in read-heavy workloads.
- Choosing synchronization primitives without considering access patterns can introduce unnecessary overhead.
