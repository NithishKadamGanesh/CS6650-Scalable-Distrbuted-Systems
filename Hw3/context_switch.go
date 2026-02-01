package main

import (
	"fmt"
	"runtime"
	"time"
)

const iterations = 1_000_000

func pingPong() time.Duration {
	ch := make(chan struct{})

	start := time.Now()

	go func() {
		for i := 0; i < iterations; i++ {
			ch <- struct{}{}
			<-ch
		}
	}()

	for i := 0; i < iterations; i++ {
		<-ch
		ch <- struct{}{}
	}

	return time.Since(start)
}

func main() {
	// Case 1: single OS thread
	runtime.GOMAXPROCS(1)
	durationSingle := pingPong()

	// Case 2: multiple OS threads
	runtime.GOMAXPROCS(runtime.NumCPU())
	durationMulti := pingPong()

	avgSingle := durationSingle / time.Duration(2*iterations)
	avgMulti := durationMulti / time.Duration(2*iterations)

	fmt.Println("Single OS thread:")
	fmt.Println("  Total time:", durationSingle)
	fmt.Println("  Avg handoff time:", avgSingle)

	fmt.Println("Multiple OS threads:")
	fmt.Println("  Total time:", durationMulti)
	fmt.Println("  Avg handoff time:", avgMulti)
}