package app

type inputFocus int

const (
	focusSidebar inputFocus = iota
	focusChatInput
)

type InputController struct {
	focus inputFocus
}

func NewInputController() *InputController {
	return &InputController{focus: focusSidebar}
}

func (c *InputController) FocusSidebar() {
	c.focus = focusSidebar
}

func (c *InputController) FocusChatInput() {
	c.focus = focusChatInput
}

func (c *InputController) IsChatFocused() bool {
	return c.focus == focusChatInput
}

func (c *InputController) IsSidebarFocused() bool {
	return c.focus == focusSidebar
}
