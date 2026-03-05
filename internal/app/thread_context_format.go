package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func formatTokensOrDash(tokens *int64) string {
	if tokens == nil || *tokens < 0 {
		return "-"
	}
	return fmt.Sprintf("%s tokens", formatIntWithCommas(*tokens))
}

func formatContextUsedOrDash(percent *float64) string {
	if percent == nil || math.IsNaN(*percent) || math.IsInf(*percent, 0) || *percent < 0 {
		return "-"
	}
	return fmt.Sprintf("%d%% used", int(math.Round(*percent)))
}

func formatSpendOrDash(spendUSD *float64) string {
	if spendUSD == nil || math.IsNaN(*spendUSD) || math.IsInf(*spendUSD, 0) || *spendUSD < 0 {
		return "-"
	}
	return fmt.Sprintf("$%.2f spend", *spendUSD)
}

func formatIntWithCommas(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}
	raw := strconv.FormatInt(value, 10)
	if len(raw) <= 3 {
		if negative {
			return "-" + raw
		}
		return raw
	}
	var b strings.Builder
	if negative {
		b.WriteByte('-')
	}
	for i, r := range raw {
		if i > 0 && (len(raw)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
