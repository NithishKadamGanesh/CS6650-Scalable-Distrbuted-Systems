package resilience

import (
	"testing"
	"time"
)

func TestCircuitBreakerStateMachine(t *testing.T) {
	cb := New("test-cb-sm", Config{FailureThreshold: 3, Cooldown: 50 * time.Millisecond, RecoveryTarget: 2})

	if !cb.Allow() { t.Fatal("new breaker should allow") }
	if cb.State() != StateClosed { t.Fatal("expected CLOSED initially") }

	cb.RecordFailure(); cb.RecordFailure(); cb.RecordFailure()
	if cb.State() != StateOpen { t.Fatalf("expected OPEN, got %s", cb.State()) }
	if cb.Allow() { t.Fatal("OPEN breaker should reject") }

	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() { t.Fatal("HALF-OPEN should allow one probe") }
	if cb.State() != StateHalfOpen { t.Fatalf("expected HALF-OPEN, got %s", cb.State()) }

	cb.RecordSuccess(); cb.RecordSuccess()
	if cb.State() != StateClosed { t.Fatalf("expected CLOSED after recovery, got %s", cb.State()) }
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	cb := New("test-cb-half", Config{FailureThreshold: 1, Cooldown: 10 * time.Millisecond})
	cb.RecordFailure()
	time.Sleep(15 * time.Millisecond)
	cb.Allow()
	if cb.State() != StateHalfOpen { t.Fatal("expected HALF-OPEN") }
	cb.RecordFailure()
	if cb.State() != StateOpen { t.Fatal("HALF-OPEN failure should flip back to OPEN") }
}

func TestCircuitBreakerSuccessResetsFailures(t *testing.T) {
	cb := New("test-cb-reset", Config{FailureThreshold: 3, Cooldown: 10 * time.Millisecond})
	cb.RecordFailure(); cb.RecordFailure()
	cb.RecordSuccess()
	cb.RecordFailure(); cb.RecordFailure()
	if cb.State() != StateClosed { t.Fatal("success should reset failure counter") }
}

func TestCircuitBreakerCallback(t *testing.T) {
	var transitions []string
	cb := New("test-cb-cb", Config{
		FailureThreshold: 1, Cooldown: 10 * time.Millisecond, RecoveryTarget: 1,
		OnStateChange: func(name string, from, to State) {
			transitions = append(transitions, from.String()+"→"+to.String())
		},
	})
	cb.RecordFailure()
	time.Sleep(15 * time.Millisecond)
	cb.Allow()
	cb.RecordSuccess()
	time.Sleep(10 * time.Millisecond)

	if len(transitions) != 3 { t.Fatalf("expected 3 transitions, got %v", transitions) }
}
