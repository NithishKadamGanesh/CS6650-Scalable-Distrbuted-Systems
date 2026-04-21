package resilience

import (
	"errors"
	"testing"
)

func TestTryPrimarySuccess(t *testing.T) {
	b := NewBuffer(10)
	if !TryPrimary(b, "data", func() error { return nil }) { t.Fatal("expected success") }
	if b.Size() != 0 { t.Fatal("buffer should be empty") }
}

func TestTryPrimaryFallsBackToBuffer(t *testing.T) {
	b := NewBuffer(10)
	if TryPrimary(b, "x", func() error { return errors.New("down") }) { t.Fatal("expected failure") }
	if b.Size() != 1 { t.Fatalf("expected 1 entry, got %d", b.Size()) }
}

func TestBufferBoundedSize(t *testing.T) {
	b := NewBuffer(3)
	for i := 0; i < 10; i++ { b.Add(i) }
	if b.Size() != 3 { t.Fatalf("expected 3, got %d", b.Size()) }

	entries := b.Drain()
	if len(entries) != 3 { t.Fatal("expected 3 drained") }
	for i, e := range entries {
		if e.Payload.(int) != i+7 { t.Errorf("entry %d: expected %d, got %v", i, i+7, e.Payload) }
	}
}

func TestDrainEmpties(t *testing.T) {
	b := NewBuffer(10)
	b.Add("a"); b.Add("b")
	entries := b.Drain()
	if len(entries) != 2 { t.Fatal("expected 2 entries") }
	if b.Size() != 0 { t.Fatal("buffer should be empty after drain") }
}
