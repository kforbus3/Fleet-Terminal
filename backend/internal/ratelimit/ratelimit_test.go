package ratelimit

import "testing"

func TestLimiterAllowsBurstThenBlocks(t *testing.T) {
	// 60/min = 1/sec, burst 3. Three immediate requests pass, the fourth blocks
	// (refill within the same instant is negligible).
	l := New(60, 3)
	key := "1.2.3.4"
	for i := 0; i < 3; i++ {
		if !l.Allow(key) {
			t.Fatalf("request %d should be allowed within burst", i+1)
		}
	}
	if l.Allow(key) {
		t.Fatal("fourth request should be blocked after burst is exhausted")
	}
}

func TestLimiterIsolatesKeys(t *testing.T) {
	l := New(60, 1)
	if !l.Allow("a") {
		t.Fatal("first request for key a should pass")
	}
	if l.Allow("a") {
		t.Fatal("second request for key a should block")
	}
	// A different key has its own independent bucket.
	if !l.Allow("b") {
		t.Fatal("first request for key b should pass independently")
	}
}

func TestLimiterDisabledWhenZero(t *testing.T) {
	l := New(0, 0)
	if l.Enabled() {
		t.Fatal("limiter with 0/min should be disabled")
	}
	for i := 0; i < 100; i++ {
		if !l.Allow("x") {
			t.Fatal("disabled limiter must allow every request")
		}
	}
}
