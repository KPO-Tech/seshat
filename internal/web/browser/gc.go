package browser

import "time"

func (m *RodManager) startReaper() {
	interval := m.config.SessionReaperInterval
	if interval <= 0 {
		return
	}
	m.reaperDone = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(m.reaperDone)
		for {
			select {
			case <-ticker.C:
				m.reapExpiredSessions()
			case <-m.closeCh:
				return
			}
		}
	}()
}

func (m *RodManager) reapExpiredSessions() {
	now := time.Now().UTC()
	m.mu.Lock()
	expired := make([]*sessionState, 0)
	for sessionID, session := range m.sessions {
		if !m.sessionExpired(now, session) {
			continue
		}
		expired = append(expired, session)
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	for _, session := range expired {
		_ = m.shutdownSession(session)
	}
}

func (m *RodManager) sessionExpired(now time.Time, session *sessionState) bool {
	if session == nil {
		return false
	}
	if m.config.MaxSessionAge > 0 && now.Sub(session.createdAt) > m.config.MaxSessionAge {
		return true
	}
	if m.config.MaxIdleSession > 0 && now.Sub(session.lastActivity) > m.config.MaxIdleSession {
		return true
	}
	return false
}
