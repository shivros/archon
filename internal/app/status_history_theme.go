package app

const (
	statusHistoryListVisibleRows = 7
)

type StatusHistoryOverlayConfig struct {
	RowTruncateWidth int
	PanelMinWidth    int
	PanelMaxWidth    int
}

func defaultStatusHistoryOverlayConfig() StatusHistoryOverlayConfig {
	return StatusHistoryOverlayConfig{
		RowTruncateWidth: 64,
		PanelMinWidth:    38,
		PanelMaxWidth:    78,
	}
}
