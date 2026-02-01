package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

const iterations = 100000

func unbufferedWrite(filename string) time.Duration {
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	start := time.Now()

	for i := 0; i < iterations; i++ {
		line := fmt.Sprintf("This is line %d\n", i)
		_, err := f.Write([]byte(line))
		if err != nil {
			panic(err)
		}
	}

	return time.Since(start)
}

func bufferedWrite(filename string) time.Duration {
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)

	start := time.Now()

	for i := 0; i < iterations; i++ {
		line := fmt.Sprintf("This is line %d\n", i)
		_, err := writer.WriteString(line)
		if err != nil {
			panic(err)
		}
	}

	err = writer.Flush()
	if err != nil {
		panic(err)
	}

	return time.Since(start)
}

func main() {
	unbufferedTime := unbufferedWrite("unbuffered.txt")
	bufferedTime := bufferedWrite("buffered.txt")

	fmt.Println("Unbuffered write time:", unbufferedTime)
	fmt.Println("Buffered write time:", bufferedTime)
}