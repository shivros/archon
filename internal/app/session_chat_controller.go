package app

import tea "github.com/charmbracelet/bubbletea"

type SessionChatController struct {
	api         SessionChatAPI
	codexStream *CodexStreamController
}

func NewSessionChatController(api SessionChatAPI, codexStream *CodexStreamController) *SessionChatController {
	return &SessionChatController{api: api, codexStream: codexStream}
}

func (c *SessionChatController) SendMessage(sessionID, text string) tea.Cmd {
	if c == nil || c.api == nil {
		return nil
	}
	return sendSessionCmd(c.api, sessionID, text, 0)
}

func (c *SessionChatController) OpenEventStream(sessionID string) tea.Cmd {
	if c == nil || c.api == nil {
		return nil
	}
	return openEventsCmd(c.api, sessionID)
}

func (c *SessionChatController) CloseEventStream() {
	if c == nil || c.codexStream == nil {
		return
	}
	c.codexStream.Reset()
}
