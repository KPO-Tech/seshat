package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// The browser core keeps one background watcher per incognito browser context so
// pages opened by window.open/target=_blank are folded into the same deterministic
// Nexus page set without forcing the agent to discover them manually.
func (m *RodManager) startSessionTargetWatcher(session *sessionState) {
	if session == nil || session.incognito == nil || session.watchCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	session.watchCancel = cancel

	go session.incognito.Context(ctx).EachEvent(
		func(e *proto.TargetTargetCreated) {
			m.onTargetCreated(session, e.TargetInfo)
		},
		func(e *proto.TargetTargetDestroyed) {
			m.onTargetDestroyed(session, string(e.TargetID))
		},
		func(e *proto.TargetTargetInfoChanged) {
			m.onTargetInfoChanged(session, e.TargetInfo)
		},
	)()
}

func (m *RodManager) onTargetCreated(session *sessionState, target *proto.TargetTargetInfo) {
	if !m.matchesSessionContext(session, target) || target.Type != proto.TargetTargetInfoTypePage {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	_, _ = m.ensurePageStateFromTargetLocked(session, string(target.TargetID))
}

func (m *RodManager) onTargetDestroyed(session *sessionState, targetID string) {
	if session == nil || targetID == "" {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if pageID, ok := session.pageTargets[targetID]; ok {
		session.removePage(pageID)
	}
}

func (m *RodManager) onTargetInfoChanged(session *sessionState, target *proto.TargetTargetInfo) {
	if !m.matchesSessionContext(session, target) || target.Type != proto.TargetTargetInfoTypePage {
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()

	pageID, ok := session.pageTargets[string(target.TargetID)]
	if !ok {
		return
	}
	state := session.pages[pageID]
	if state == nil {
		return
	}
	state.info.URL = target.URL
	state.info.Title = target.Title
	state.info.UpdatedAt = time.Now().UTC()
}

func (m *RodManager) matchesSessionContext(session *sessionState, target *proto.TargetTargetInfo) bool {
	return session != nil &&
		target != nil &&
		session.incognito != nil &&
		target.BrowserContextID == session.incognito.BrowserContextID
}

func (m *RodManager) reconcilePagesLocked(session *sessionState) error {
	if session == nil || session.incognito == nil {
		return nil
	}

	pages, err := withRodResult(func() (rod.Pages, error) {
		return session.incognito.Pages()
	})
	if err != nil {
		return err
	}

	seenTargets := make(map[string]bool, len(pages))
	for _, page := range pages {
		if page == nil {
			continue
		}
		state, stateErr := m.ensurePageStateLocked(session, page)
		if stateErr != nil {
			return stateErr
		}
		seenTargets[state.targetID] = true
	}

	for targetID, pageID := range session.pageTargets {
		if !seenTargets[targetID] {
			session.removePage(pageID)
		}
	}

	return nil
}

func (m *RodManager) ensurePageStateLocked(session *sessionState, page *rod.Page) (*pageState, error) {
	if page == nil {
		return nil, fmt.Errorf("missing browser page")
	}

	info, err := withRodResult(func() (*proto.TargetTargetInfo, error) {
		return page.Info()
	})
	if err != nil {
		return nil, err
	}
	return m.ensurePageStateFromInfoLocked(session, page, info)
}

func (m *RodManager) ensurePageStateFromTargetLocked(session *sessionState, targetID string) (*pageState, error) {
	if pageID, ok := session.pageTargets[targetID]; ok {
		return session.pages[pageID], nil
	}

	page, err := withRodResult(func() (*rod.Page, error) {
		return session.incognito.PageFromTarget(proto.TargetTargetID(targetID))
	})
	if err != nil {
		return nil, err
	}
	info, err := withRodResult(func() (*proto.TargetTargetInfo, error) {
		return page.Info()
	})
	if err != nil {
		return nil, err
	}
	return m.ensurePageStateFromInfoLocked(session, page, info)
}

func (m *RodManager) ensurePageStateFromInfoLocked(session *sessionState, page *rod.Page, info *proto.TargetTargetInfo) (*pageState, error) {
	if info == nil {
		return nil, fmt.Errorf("missing browser page info")
	}
	targetID := string(info.TargetID)
	if pageID, ok := session.pageTargets[targetID]; ok {
		state := session.pages[pageID]
		if state != nil {
			state.page = page
			state.targetID = targetID
			state.info.URL = info.URL
			state.info.Title = info.Title
			state.info.UpdatedAt = time.Now().UTC()
			return state, nil
		}
	}

	now := time.Now().UTC()
	state := &pageState{
		info: PageInfo{
			ID:        session.allocatePageID(),
			URL:       info.URL,
			Title:     info.Title,
			CreatedAt: now,
			UpdatedAt: now,
		},
		page:     page,
		targetID: targetID,
		domRefs:  map[string]domRef{},
	}
	session.addPage(state)
	m.attachPageNetworkWatcherLocked(session, state)
	if err := applyNetworkPolicyToPage(state, session.networkPolicy); err != nil {
		return nil, err
	}
	return state, nil
}
