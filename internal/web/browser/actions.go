package browser

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Click activates a snapshot-backed interactive element on the page.
func (m *RodManager) Click(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string) (PageInfo, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return PageInfo{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if err := m.beforeActionLocked(session, "click:"+pageState.info.ID+":"+strings.TrimSpace(elementID)); err != nil {
		return PageInfo{}, err
	}
	if strings.TrimSpace(elementID) == "" {
		return PageInfo{}, fmt.Errorf("element_id is required")
	}
	ref, err := pageState.selectorForAction(elementID, revision, false)
	if err != nil {
		return PageInfo{}, err
	}
	if err := m.clickElement(pageState.page, ref.SelectorHint); err != nil {
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

// Type writes text into a snapshot-backed editable element.
func (m *RodManager) Type(ctx context.Context, sessionID types.SessionID, pageID string, elementID string, revision string, text string, clear bool) (PageInfo, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return PageInfo{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if err := m.beforeActionLocked(session, "type:"+pageState.info.ID+":"+strings.TrimSpace(elementID)+":"+fmt.Sprintf("%t", clear)); err != nil {
		return PageInfo{}, err
	}
	if strings.TrimSpace(elementID) == "" {
		return PageInfo{}, fmt.Errorf("element_id is required")
	}
	ref, err := pageState.selectorForAction(elementID, revision, true)
	if err != nil {
		return PageInfo{}, err
	}
	if err := m.typeIntoElement(pageState.page, ref.SelectorHint, text, clear); err != nil {
		return PageInfo{}, err
	}

	pageState.invalidateDOMRefs()
	pageState.info = m.refreshPageInfo(pageState.page, pageState.info)
	session.setActivePage(pageState.info.ID)
	return pageState.info, nil
}

// Press sends a keyboard key to the page.
func (m *RodManager) Press(ctx context.Context, sessionID types.SessionID, pageID string, key string) (PageInfo, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return PageInfo{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if err := m.beforeActionLocked(session, "press:"+pageState.info.ID+":"+strings.TrimSpace(key)); err != nil {
		return PageInfo{}, err
	}
	if err := m.pressKey(pageState.page, key); err != nil {
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

// Scroll moves the page viewport by a directional amount.
func (m *RodManager) Scroll(ctx context.Context, sessionID types.SessionID, pageID string, options ScrollOptions) (PageInfo, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return PageInfo{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if err := m.beforeActionLocked(session, "scroll:"+pageState.info.ID+":"+strings.TrimSpace(options.Direction)+":"+fmt.Sprintf("%d", options.Amount)); err != nil {
		return PageInfo{}, err
	}
	if err := m.scrollPage(pageState.page, options); err != nil {
		return PageInfo{}, err
	}

	pageState.invalidateDOMRefs()
	pageState.info = m.refreshPageInfo(pageState.page, pageState.info)
	session.setActivePage(pageState.info.ID)
	return pageState.info, nil
}

// Wait blocks until the page stabilizes and optionally contains text.
func (m *RodManager) Wait(ctx context.Context, sessionID types.SessionID, pageID string, options WaitOptions) (PageInfo, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return PageInfo{}, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if err := m.beforeActionLocked(session, "wait:"+pageState.info.ID+":"+strings.TrimSpace(options.Text)); err != nil {
		return PageInfo{}, err
	}
	if err := m.waitForCondition(ctx, pageState.page, options); err != nil {
		return PageInfo{}, err
	}

	pageState.info = m.refreshPageInfo(pageState.page, pageState.info)
	session.setActivePage(pageState.info.ID)
	return pageState.info, nil
}

func (m *RodManager) clickElement(page *rod.Page, selector string) error {
	_, err := withRodResult(func() (string, error) {
		result, evalErr := page.Evaluate(rod.Eval(`
(selector) => {
  const node = document.querySelector(String(selector));
  if (!node) throw new Error("element not found");
  node.click();
  return location.href;
}`, selector))
		if evalErr != nil {
			return "", evalErr
		}
		return result.Value.String(), nil
	})
	return err
}

func (m *RodManager) typeIntoElement(page *rod.Page, selector string, text string, clear bool) error {
	_, err := withRodResult(func() (string, error) {
		result, evalErr := page.Evaluate(rod.Eval(`
(selector, text, clearInput) => {
  const node = document.querySelector(String(selector));
  if (!node) throw new Error("element not found");
  node.focus();
  if ('value' in node) {
    if (clearInput) node.value = "";
    node.value = String(clearInput ? text : String(node.value || "") + text);
    node.dispatchEvent(new Event('input', { bubbles: true }));
    node.dispatchEvent(new Event('change', { bubbles: true }));
  } else {
    if (clearInput) node.textContent = "";
    node.textContent = String(clearInput ? text : String(node.textContent || "") + text);
    node.dispatchEvent(new Event('input', { bubbles: true }));
  }
  return location.href;
}`, selector, text, clear))
		if evalErr != nil {
			return "", evalErr
		}
		return result.Value.String(), nil
	})
	return err
}

func (m *RodManager) pressKey(page *rod.Page, key string) error {
	mappedKey, err := normalizeKey(key)
	if err != nil {
		return err
	}
	return withRod(func() error {
		return page.Keyboard.Type(mappedKey)
	})
}

func (m *RodManager) scrollPage(page *rod.Page, options ScrollOptions) error {
	amount := options.Amount
	if amount == 0 {
		amount = 600
	}
	direction := strings.TrimSpace(strings.ToLower(options.Direction))
	if direction == "" {
		direction = "down"
	}

	dx, dy := 0, 0
	switch direction {
	case "down":
		dy = amount
	case "up":
		dy = -amount
	case "right":
		dx = amount
	case "left":
		dx = -amount
	default:
		return fmt.Errorf("unsupported scroll direction %q", options.Direction)
	}

	_, err := withRodResult(func() (string, error) {
		result, evalErr := page.Evaluate(rod.Eval(`
(dx, dy) => {
  window.scrollBy(Number(dx), Number(dy));
  return location.href;
}`, dx, dy))
		if evalErr != nil {
			return "", evalErr
		}
		return result.Value.String(), nil
	})
	return err
}

func (m *RodManager) waitForCondition(ctx context.Context, page *rod.Page, options WaitOptions) error {
	timeout := time.Duration(options.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = m.config.NavigationTimeout
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if strings.TrimSpace(options.Text) == "" {
		return withRod(func() error {
			if err := page.WaitLoad(); err != nil {
				return err
			}
			return page.WaitStable(m.config.WaitAfterNavigation)
		})
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	want := strings.TrimSpace(options.Text)
	for {
		select {
		case <-deadlineCtx.Done():
			return fmt.Errorf("browser wait timed out waiting for text %q", want)
		case <-ticker.C:
			ok, err := withRodResult(func() (bool, error) {
				result, evalErr := page.Evaluate(rod.Eval(`
(needle) => {
  const body = document.body ? document.body.innerText || "" : "";
  return body.includes(String(needle));
}`, want))
				if evalErr != nil {
					return false, evalErr
				}
				return result.Value.Bool(), nil
			})
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
		}
	}
}

func (m *RodManager) refreshPageInfo(page *rod.Page, previous PageInfo) PageInfo {
	info := previous
	if pageInfo, err := withRodResult(func() (*proto.TargetTargetInfo, error) {
		return page.Info()
	}); err == nil && pageInfo != nil {
		info.URL = pageInfo.URL
		info.Title = pageInfo.Title
	}
	info.UpdatedAt = time.Now().UTC()
	return info
}

func normalizeKey(raw string) (input.Key, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "enter":
		return input.Enter, nil
	case "tab":
		return input.Tab, nil
	case "escape", "esc":
		return input.Escape, nil
	case "space":
		return input.Space, nil
	case "backspace":
		return input.Backspace, nil
	case "arrowdown", "down":
		return input.ArrowDown, nil
	case "arrowup", "up":
		return input.ArrowUp, nil
	case "arrowleft", "left":
		return input.ArrowLeft, nil
	case "arrowright", "right":
		return input.ArrowRight, nil
	case "pagedown":
		return input.PageDown, nil
	case "pageup":
		return input.PageUp, nil
	default:
		return 0, fmt.Errorf("unsupported key %q", raw)
	}
}
