package app

import "testing"

func TestChatBubbleStylesUseSharedSymmetricPadding(t *testing.T) {
	styles := []struct {
		name  string
		style interface {
			GetPaddingTop() int
			GetPaddingBottom() int
			GetPaddingLeft() int
			GetPaddingRight() int
		}
	}{
		{name: "user", style: userBubbleStyle},
		{name: "agent", style: agentBubbleStyle},
		{name: "system", style: systemBubbleStyle},
		{name: "reasoning", style: reasoningBubbleStyle},
		{name: "approval", style: approvalBubbleStyle},
		{name: "approvalResolved", style: approvalResolvedBubbleStyle},
	}

	for _, tc := range styles {
		if got := tc.style.GetPaddingTop(); got != chatBubblePaddingVertical {
			t.Fatalf("%s padding top: expected %d, got %d", tc.name, chatBubblePaddingVertical, got)
		}
		if got := tc.style.GetPaddingBottom(); got != chatBubblePaddingVertical {
			t.Fatalf("%s padding bottom: expected %d, got %d", tc.name, chatBubblePaddingVertical, got)
		}
		if got := tc.style.GetPaddingLeft(); got != chatBubblePaddingHorizontal {
			t.Fatalf("%s padding left: expected %d, got %d", tc.name, chatBubblePaddingHorizontal, got)
		}
		if got := tc.style.GetPaddingRight(); got != chatBubblePaddingHorizontal {
			t.Fatalf("%s padding right: expected %d, got %d", tc.name, chatBubblePaddingHorizontal, got)
		}
	}
}
