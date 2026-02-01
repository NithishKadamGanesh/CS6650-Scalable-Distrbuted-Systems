package main

import (
	"fmt"
	"sync"
	"time"
)

type SafeMap struct {
	mu sync.RWMutex
	m  map[int]int
}

func main() {
	safeMap := SafeMap{
		m: make(map[int]int),
	}

	start := time.Now()
	var wg sync.WaitGroup

	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				key := g*1000 + i

				safeMap.mu.Lock()
				safeMap.m[key] = i
				safeMap.mu.Unlock()
			}
		}(g)
	}

	wg.Wait()
	elapsed := time.Since(start)

	fmt.Println("Map size:", len(safeMap.m))
	fmt.Println("Time taken:", elapsed)
}