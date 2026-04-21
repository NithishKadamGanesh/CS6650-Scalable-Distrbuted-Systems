package main

import (
	"crypto/rand"
	"fmt"
)

func newUUID() string {
	var data [16]byte
	_, _ = rand.Read(data[:])
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		data[0:4],
		data[4:6],
		data[6:8],
		data[8:10],
		data[10:16],
	)
}
