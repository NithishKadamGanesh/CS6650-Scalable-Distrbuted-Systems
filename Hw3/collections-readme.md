# Collections Experiment Report

## Concurrent Writes to a Shared Map in Go

## Experiment Setup

A shared `map[int]int` was created and accessed concurrently by 50 goroutines. Each goroutine performed 1,000 iterations and inserted unique key-value pairs into the map using:

```go
m[g*1000 + i] = i
```

After all goroutines completed, the program attempted to print `len(m)`. The expected size of the map was 50,000.

## Experimental Results

The program was executed three times with identical code.

| Run | Outcome |
|-----|---------|
| 1 | Program terminated with `fatal error: concurrent map writes` |
| 2 | Program terminated with `fatal error: concurrent map writes` |
| 3 | Program terminated with `fatal error: concurrent map writes` |

In all executions, the program crashed before reaching the measurement point. As a result, no numeric map size was produced.

![Concurrent map writes error](./images/s4.png)

## Explanation of Behavior

Go maps are not safe for concurrent writes, even when goroutines write to distinct keys. Each write operation may modify shared internal structures such as hash buckets, resize metadata, and rehashing state. When multiple goroutines attempt to mutate the map simultaneously, these internal invariants are violated.

Go detects this condition at runtime and terminates the program to prevent memory corruption and undefined behavior.

## Concepts Learned

- Shared collections require synchronization even when logical key access does not overlap.
- Concurrency bugs can manifest as catastrophic system failure, not just incorrect results.
- Safety must apply to the internal structure of a data structure, not only to the values stored.

## Evidence Supporting These Concepts

- Reproducible runtime crashes across multiple executions
- Explicit error message indicating concurrent map writes
- Stack traces showing internal map rehashing during concurrent access
