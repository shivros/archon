package types

const (
	KeyActionToggleSidebar = "toggle_sidebar"
	KeyActionRefresh       = "refresh"
	KeyActionKill          = "kill"
	KeyActionQuit          = "quit"
	KeyActionMoveDown      = "move_down"
	KeyActionMoveUp        = "move_up"
	KeyActionPause         = "pause"
)

type Keymap struct {
	Bindings map[string]string `json:"bindings"`
}

func DefaultKeymap() *Keymap {
	return &Keymap{
		Bindings: map[string]string{
			KeyActionToggleSidebar: "ctrl+b",
			KeyActionRefresh:       "r",
			KeyActionKill:          "x",
			KeyActionQuit:          "q",
			KeyActionMoveDown:      "j",
			KeyActionMoveUp:        "k",
			KeyActionPause:         "p",
		},
	}
}
