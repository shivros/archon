package daemon

import "time"

type DebugBatchPolicy struct {
	FlushInterval  time.Duration
	MaxBatchBytes  int
	FlushOnNewline bool
}

func defaultDebugBatchPolicy() DebugBatchPolicy {
	return DebugBatchPolicy{
		FlushInterval:  40 * time.Millisecond,
		MaxBatchBytes:  2048,
		FlushOnNewline: true,
	}
}

func (p DebugBatchPolicy) normalize() DebugBatchPolicy {
	if p.FlushInterval < 0 {
		p.FlushInterval = 0
	}
	if p.MaxBatchBytes < 0 {
		p.MaxBatchBytes = 0
	}
	return p
}

type DebugRetentionPolicy struct {
	MaxEvents int
	MaxBytes  int
}

func defaultDebugRetentionPolicy() DebugRetentionPolicy {
	return DebugRetentionPolicy{
		MaxEvents: debugMaxEvents,
		MaxBytes:  debugMaxBufferedBytes,
	}
}

func (p DebugRetentionPolicy) normalize() DebugRetentionPolicy {
	if p.MaxEvents <= 0 {
		p.MaxEvents = debugMaxEvents
	}
	if p.MaxBytes < 0 {
		p.MaxBytes = 0
	}
	return p
}
