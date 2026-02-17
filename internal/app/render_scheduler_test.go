package app

import (
	"testing"
	"time"
)

func TestThrottledRenderSchedulerDelaysPendingRenders(t *testing.T) {
	scheduler := NewThrottledRenderScheduler(200 * time.Millisecond)
	now := time.Now()

	if !scheduler.Request(now) {
		t.Fatalf("expected first render request to be allowed immediately")
	}
	scheduler.MarkRendered(now)

	if scheduler.Request(now.Add(50 * time.Millisecond)) {
		t.Fatalf("expected render request inside interval to be deferred")
	}
	if scheduler.ShouldRender(now.Add(150 * time.Millisecond)) {
		t.Fatalf("expected deferred render to stay pending before interval passes")
	}
	if !scheduler.ShouldRender(now.Add(220 * time.Millisecond)) {
		t.Fatalf("expected deferred render once interval has passed")
	}
}

func TestThrottledRenderSchedulerDisablesThrottleForZeroInterval(t *testing.T) {
	scheduler := NewThrottledRenderScheduler(0)
	now := time.Now()

	if !scheduler.Request(now) {
		t.Fatalf("expected immediate render for zero interval")
	}
	scheduler.MarkRendered(now)
	if !scheduler.Request(now.Add(time.Millisecond)) {
		t.Fatalf("expected no throttling when interval is zero")
	}
}
