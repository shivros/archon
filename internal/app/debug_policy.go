package app

type DebugStreamRetentionPolicy struct {
	MaxLines int
	MaxBytes int
}

func defaultDebugStreamRetentionPolicy() DebugStreamRetentionPolicy {
	return DebugStreamRetentionPolicy{
		MaxLines: maxDebugViewportLines,
		MaxBytes: maxDebugViewportBytes,
	}
}

func (p DebugStreamRetentionPolicy) normalize() DebugStreamRetentionPolicy {
	if p.MaxLines < 0 {
		p.MaxLines = 0
	}
	if p.MaxBytes < 0 {
		p.MaxBytes = 0
	}
	return p
}
