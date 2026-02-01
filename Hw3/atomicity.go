package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

func nonAtomicCounter() {
	var counter int
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				counter++ // NOT atomic
			}
		}()
	}

	wg.Wait()
	fmt.Println("Non-atomic counter value:", counter)
}

func atomicCounter() {
	var counter int64
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				atomic.AddInt64(&counter, 1) // atomic
			}
		}()
	}

	wg.Wait()
	fmt.Println("Atomic counter value:", counter)
}

func main() {
	fmt.Println("Running non-atomic version")
	nonAtomicCounter()

	fmt.Println("Running atomic version")
	atomicCounter()
}
