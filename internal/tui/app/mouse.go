package app

import (
	tea "charm.land/bubbletea/v2"
)

func pointInRect(x, y, rx, ry, rw, rh int) bool {
	return x >= rx && x < rx+rw && y >= ry && y < ry+rh
}

func clampMouse(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m *Model) handleMouseClick(msg tea.MouseClickMsg) tea.Cmd {
	if m.state != stateChat {
		return nil
	}
	layout := m.currentChatLayout()
	if m.skillCompletions.IsOpen() && pointInRect(msg.X, msg.Y, layout.popupX, layout.popupY, layout.popupW, layout.popupH) {
		if msg.Button == tea.MouseLeft {
			row := msg.Y - layout.popupY - 1
			if sel := m.skillCompletions.ClickRow(row); sel != "" {
				m.input.SetValue(sel + " ")
				m.input.CursorEnd()
				m.skillCompletions.Close()
				m.focus = uiFocusEditor
				*m = m.resizeInput()
				return m.input.Focus()
			}
		}
		return nil
	}
	if pointInRect(msg.X, msg.Y, layout.inputX, layout.inputY+layout.popupH, layout.inputW, layout.inputH-layout.popupH) {
		m.focus = uiFocusEditor
		return m.input.Focus()
	}
	if !pointInRect(msg.X, msg.Y, layout.chatX, layout.chatY, layout.chatW, layout.chatH) {
		return nil
	}
	if msg.Button == tea.MouseRight {
		if text := m.chat.SelectedText(); text != "" {
			return m.copyToClipboard(text, "Selection copied")
		}
		return nil
	}
	if msg.Button != tea.MouseLeft {
		return nil
	}
	m.focus = uiFocusMain
	m.input.Blur()
	m.chat.HandleMouseDown(msg.X-layout.chatX, msg.Y-layout.chatY)
	return nil
}

func (m *Model) handleMouseMotion(msg tea.MouseMotionMsg) bool {
	if m.state != stateChat || !m.chat.HasMouseCapture() {
		return false
	}
	layout := m.currentChatLayout()
	relX := msg.X - layout.chatX
	relY := msg.Y - layout.chatY
	return m.chat.HandleMouseDrag(relX, relY)
}

func (m *Model) handleMouseRelease(msg tea.MouseReleaseMsg) tea.Cmd {
	if m.state != stateChat || !m.chat.HasMouseCapture() {
		return nil
	}
	layout := m.currentChatLayout()
	relX := msg.X - layout.chatX
	relY := msg.Y - layout.chatY
	if text := m.chat.HandleMouseUp(relX, relY); text != "" {
		return m.copyToClipboard(text, "Selection copied")
	}
	return nil
}
