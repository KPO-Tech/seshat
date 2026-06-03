package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// EnsureSession creates browser state for a Nexus session when needed.
func (m *RodManager) EnsureSession(ctx context.Context, sessionID types.SessionID) (SessionState, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return SessionState{}, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.snapshot(), nil
}

// OpenPage opens a new page within the session's isolated browser context.
func (m *RodManager) OpenPage(ctx context.Context, sessionID types.SessionID, target string) (PageInfo, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return PageInfo{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if err := m.beforeActionLocked(session, "open:"+strings.TrimSpace(target)); err != nil {
		return PageInfo{}, err
	}
	if len(session.pages) >= m.config.MaxPagesPerSession {
		if err := m.evictOldestInactivePageLocked(session); err != nil {
			return PageInfo{}, fmt.Errorf("browser session page limit reached (%d)", m.config.MaxPagesPerSession)
		}
	}

	targetURL := normalizeURL(target)
	if err := validateNavigationURL(targetURL); err != nil {
		return PageInfo{}, err
	}
	page, err := withRodResult(func() (*rod.Page, error) {
		return session.incognito.Page(proto.TargetCreateTarget{URL: targetURL})
	})
	if err != nil {
		return PageInfo{}, err
	}
	if err := m.waitForPageReady(page); err != nil {
		return PageInfo{}, err
	}

	state, err := m.ensurePageStateLocked(session, page)
	if err != nil {
		return PageInfo{}, err
	}
	state.info = m.refreshPageInfo(page, state.info)
	session.setActivePage(state.info.ID)
	return state.info, nil
}

// Navigate loads the provided URL into an existing page.
func (m *RodManager) Navigate(ctx context.Context, sessionID types.SessionID, pageID string, target string) (PageInfo, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return PageInfo{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if err := m.beforeActionLocked(session, "navigate:"+pageState.info.ID+":"+strings.TrimSpace(target)); err != nil {
		return PageInfo{}, err
	}
	targetURL := normalizeURL(target)
	if err := validateNavigationURL(targetURL); err != nil {
		return PageInfo{}, err
	}
	if err := withRod(func() error {
		return pageState.page.Navigate(targetURL)
	}); err != nil {
		return PageInfo{}, err
	}
	if err := m.waitForPageReady(pageState.page); err != nil {
		return PageInfo{}, err
	}

	pageState.invalidateDOMRefs()
	pageState.info = m.refreshPageInfo(pageState.page, pageState.info)
	session.setActivePage(pageState.info.ID)
	if err := m.reconcilePagesLocked(session); err != nil {
		return PageInfo{}, err
	}
	return pageState.info, nil
}

// ListPages returns metadata for pages in the session context.
func (m *RodManager) ListPages(ctx context.Context, sessionID types.SessionID) ([]PageInfo, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.lastActivity = time.Now().UTC()
	if err := m.reconcilePagesLocked(session); err != nil {
		return nil, err
	}
	return session.orderedPageInfos(), nil
}

// ListNetwork returns recent network activity for the selected page or whole session.
func (m *RodManager) ListNetwork(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]NetworkEntry, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	_ = ctx
	session.lastActivity = time.Now().UTC()
	return session.networkEntries(pageID, limit), nil
}

// ListDownloads returns recent download activity for the selected page or whole session.
func (m *RodManager) ListDownloads(ctx context.Context, sessionID types.SessionID, pageID string, limit int) ([]DownloadEntry, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	_ = ctx
	session.lastActivity = time.Now().UTC()
	return session.downloadEntries(pageID, limit), nil
}

// SelectPage marks an existing page as active for subsequent page-less operations.
func (m *RodManager) SelectPage(ctx context.Context, sessionID types.SessionID, pageID string) (PageInfo, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return PageInfo{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.lastActivity = time.Now().UTC()
	pageState.info.UpdatedAt = time.Now().UTC()
	session.setActivePage(pageState.info.ID)
	return pageState.info, nil
}

// ClosePage closes a page and updates the session active page pointer.
func (m *RodManager) ClosePage(ctx context.Context, sessionID types.SessionID, pageID string) (SessionState, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return SessionState{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	session.lastActivity = time.Now().UTC()
	if err := withRod(func() error { return pageState.page.Close() }); err != nil {
		return SessionState{}, err
	}
	session.removePage(pageState.info.ID)
	return session.snapshot(), nil
}

// CloseSession tears down the isolated session browser context.
func (m *RodManager) CloseSession(ctx context.Context, sessionID types.SessionID) error {
	_ = ctx

	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if ok {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if !ok {
		return nil
	}
	return m.shutdownSession(session)
}

func (m *RodManager) ensureSession(ctx context.Context, sessionID types.SessionID) (*sessionState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensureSessionLocked(ctx, sessionID)
}

func (m *RodManager) ensureSessionLocked(ctx context.Context, sessionID types.SessionID) (*sessionState, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("browser session requires a session ID")
	}
	if existing, ok := m.sessions[sessionID]; ok {
		return existing, nil
	}
	root, err := m.ensureRootBrowserLocked(ctx)
	if err != nil {
		return nil, err
	}
	incognito, err := withRodResult(func() (*rod.Browser, error) {
		return root.Incognito()
	})
	if err != nil {
		return nil, err
	}
	session := &sessionState{
		id:           sessionID,
		createdAt:    time.Now().UTC(),
		lastActivity: time.Now().UTC(),
		incognito:    incognito,
		pages:        make(map[string]*pageState),
		pageTargets:  make(map[string]string),
		downloadByID: make(map[string]int),
		maxNetLog:    m.config.MaxNetworkEntries,
		maxDownloads: m.config.MaxDownloadEntries,
	}
	if err := m.configureSessionDownloadsLocked(session); err != nil {
		_ = withRod(func() error { return incognito.Close() })
		return nil, err
	}
	m.sessions[sessionID] = session
	m.startSessionTargetWatcher(session)
	return session, nil
}

func (m *RodManager) ensureRootBrowserLocked(ctx context.Context) (*rod.Browser, error) {
	if m.root != nil {
		return m.root, nil
	}
	launch := launcher.New()
	controlURL := strings.TrimSpace(m.config.RemoteControlURL)
	if controlURL == "" {
		if m.config.ExecutablePath != "" {
			launch = launch.Bin(m.config.ExecutablePath)
		}
		if m.config.Headless {
			launch = launch.Headless(true)
		}
		if m.config.LauncherLeakless {
			launch = launch.Leakless(true)
		}
		var err error
		controlURL, err = launch.Launch()
		if err != nil {
			return nil, fmt.Errorf("launch chrome: %w", err)
		}
	}

	browser := rod.New().ControlURL(controlURL)
	if deadline, ok := ctx.Deadline(); ok {
		browser = browser.Timeout(time.Until(deadline))
	}
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect chrome: %w", err)
	}
	m.root = browser
	m.startReaper()
	return browser, nil
}

func (m *RodManager) resolvePage(ctx context.Context, sessionID types.SessionID, pageID string) (*sessionState, *pageState, error) {
	session, err := m.ensureSession(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()

	if err := m.reconcilePagesLocked(session); err != nil {
		return nil, nil, err
	}

	resolvedPageID := strings.TrimSpace(pageID)
	if resolvedPageID == "" {
		resolvedPageID = session.activePageID
	}
	if resolvedPageID == "" {
		return nil, nil, fmt.Errorf("no active browser page")
	}
	pageState, ok := session.pages[resolvedPageID]
	if !ok {
		return nil, nil, fmt.Errorf("browser page %q not found", resolvedPageID)
	}
	return session, pageState, nil
}

func (m *RodManager) shutdownSession(session *sessionState) error {
	if session == nil {
		return nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()

	if session.watchCancel != nil {
		session.watchCancel()
		session.watchCancel = nil
	}
	for _, pageID := range append([]string(nil), session.pageOrder...) {
		if state, ok := session.pages[pageID]; ok && state.page != nil {
			_ = withRod(func() error { return state.page.Close() })
			session.removePage(pageID)
		}
	}
	if session.incognito != nil {
		_ = withRod(func() error { return session.incognito.Close() })
	}
	return nil
}

// Close releases all browser resources owned by this manager.
func (m *RodManager) Close() error {
	if m == nil {
		return nil
	}
	var closeErr error
	m.closeOnce.Do(func() {
		close(m.closeCh)
		if m.reaperDone != nil {
			<-m.reaperDone
		}

		m.mu.Lock()
		sessions := make([]*sessionState, 0, len(m.sessions))
		for sessionID, session := range m.sessions {
			sessions = append(sessions, session)
			delete(m.sessions, sessionID)
		}
		root := m.root
		m.root = nil
		m.mu.Unlock()

		for _, session := range sessions {
			if err := m.shutdownSession(session); err != nil && closeErr == nil {
				closeErr = err
			}
		}
		if root != nil {
			if err := withRod(func() error { return root.Close() }); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	})
	return closeErr
}

func (m *RodManager) configureSessionDownloadsLocked(session *sessionState) error {
	if session == nil || session.incognito == nil || m.root == nil {
		return nil
	}
	dir := m.config.DownloadDir
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join(os.TempDir(), "nexus-browser-downloads", strings.ReplaceAll(string(session.id), ":", "_"))
	} else {
		dir = filepath.Join(dir, strings.ReplaceAll(string(session.id), ":", "_"))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("prepare browser download directory: %w", err)
	}
	if err := (proto.BrowserSetDownloadBehavior{
		Behavior:         proto.BrowserSetDownloadBehaviorBehaviorAllowAndName,
		BrowserContextID: session.incognito.BrowserContextID,
		DownloadPath:     dir,
		EventsEnabled:    true,
	}).Call(m.root); err != nil {
		return fmt.Errorf("enable browser downloads: %w", err)
	}
	session.downloadDir = dir
	return nil
}

func (m *RodManager) waitForPageReady(page *rod.Page) error {
	return withRod(func() error {
		if err := page.WaitLoad(); err != nil {
			return err
		}
		return page.WaitStable(m.config.WaitAfterNavigation)
	})
}

func (m *RodManager) readPageTitle(page *rod.Page) string {
	title, err := withRodResult(func() (string, error) {
		info, infoErr := page.Info()
		if infoErr != nil {
			return "", infoErr
		}
		return info.Title, nil
	})
	if err != nil {
		return ""
	}
	return title
}

func (s *sessionState) snapshot() SessionState {
	return SessionState{
		SessionID:    s.id,
		ActivePageID: s.activePageID,
		PageCount:    len(s.pages),
		ActionCount:  s.actionCount,
		CreatedAt:    s.createdAt,
		LastActivity: s.lastActivity,
	}
}

func (m *RodManager) beforeActionLocked(session *sessionState, signature string) error {
	now := time.Now().UTC()
	if m.config.MaxSessionAge > 0 && now.Sub(session.createdAt) > m.config.MaxSessionAge {
		return fmt.Errorf("browser session exceeded maximum age of %s", m.config.MaxSessionAge)
	}
	if m.config.MaxActionsPerSession > 0 && session.actionCount >= m.config.MaxActionsPerSession {
		return fmt.Errorf("browser session action limit reached (%d)", m.config.MaxActionsPerSession)
	}

	signature = strings.TrimSpace(signature)
	if signature != "" {
		if session.lastAction == signature {
			session.repeatCount++
		} else {
			session.lastAction = signature
			session.repeatCount = 1
		}
		if m.config.MaxRepeatedAction > 0 && session.repeatCount > m.config.MaxRepeatedAction {
			return fmt.Errorf("browser repeated action limit reached for %q", signature)
		}
	}

	session.actionCount++
	session.lastActivity = now
	return nil
}

func (m *RodManager) evictOldestInactivePageLocked(session *sessionState) error {
	var oldest *pageState
	for _, pageID := range session.pageOrder {
		page := session.pages[pageID]
		if page == nil {
			continue
		}
		if page.info.Active {
			continue
		}
		if oldest == nil || page.info.UpdatedAt.Before(oldest.info.UpdatedAt) {
			oldest = page
		}
	}
	if oldest == nil {
		return fmt.Errorf("no inactive browser page available for eviction")
	}
	if err := withRod(func() error { return oldest.page.Close() }); err != nil {
		return err
	}
	session.removePage(oldest.info.ID)
	return nil
}
