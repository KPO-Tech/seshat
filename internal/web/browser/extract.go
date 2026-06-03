package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-rod/rod"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Snapshot returns a compact text + element snapshot of the selected page.
func (m *RodManager) Snapshot(ctx context.Context, sessionID types.SessionID, pageID string, options SnapshotOptions) (Snapshot, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return Snapshot{}, err
	}
	session.mu.Lock()
	if err := m.beforeActionLocked(session, "snapshot:"+pageState.info.ID); err != nil {
		session.mu.Unlock()
		return Snapshot{}, err
	}
	page := pageState.page
	info := pageState.info
	maxText := options.MaxText
	if maxText <= 0 {
		maxText = m.config.MaxSnapshotText
	}
	maxElements := options.MaxElements
	if maxElements <= 0 {
		maxElements = m.config.MaxSnapshotElements
	}
	session.mu.Unlock()

	payload, err := m.readSnapshotPayload(page, maxText, maxElements)
	if err != nil {
		return Snapshot{}, err
	}
	takenAt := time.Now().UTC()
	revision := snapshotRevision(takenAt)
	info.URL = payload.URL
	info.Title = payload.Title
	info.UpdatedAt = takenAt

	snapshot := Snapshot{
		Page:      info,
		Revision:  revision,
		Text:      payload.Text,
		Elements:  payload.Elements,
		Headings:  payload.Headings,
		TakenAt:   takenAt,
		Truncated: len(payload.Text) >= maxText,
	}

	session.mu.Lock()
	pageState.info = info
	pageState.rebuildDOMRefs(payload.Elements, revision, payload.Text, takenAt)
	pageState.lastSnapshotHeadings = append([]HeadingInfo(nil), payload.Headings...)
	session.setActivePage(pageState.info.ID)
	session.mu.Unlock()

	return snapshot, nil
}

// Screenshot captures the selected page as a base64-encoded PNG payload.
func (m *RodManager) Screenshot(ctx context.Context, sessionID types.SessionID, pageID string, options ScreenshotOptions) (Screenshot, error) {
	session, pageState, err := m.resolvePage(ctx, sessionID, pageID)
	if err != nil {
		return Screenshot{}, err
	}
	session.mu.Lock()
	if err := m.beforeActionLocked(session, "screenshot:"+pageState.info.ID+":"+fmt.Sprintf("%t", options.FullPage)); err != nil {
		session.mu.Unlock()
		return Screenshot{}, err
	}
	page := pageState.page
	info := pageState.info
	session.mu.Unlock()

	image, err := withRodResult(func() ([]byte, error) {
		return page.Screenshot(options.FullPage, nil)
	})
	if err != nil {
		return Screenshot{}, err
	}
	info = m.refreshPageInfo(page, info)
	persistedPath, err := m.persistScreenshot(ctx, sessionID, info.ID, image)
	if err != nil {
		return Screenshot{}, err
	}

	return Screenshot{
		Page:          info,
		MimeType:      "image/png",
		DataBase64:    base64.StdEncoding.EncodeToString(image),
		Bytes:         len(image),
		PersistedPath: persistedPath,
		PersistedSize: len(image),
		FullPage:      options.FullPage,
		TakenAt:       time.Now().UTC(),
	}, nil
}

func snapshotRevision(takenAt time.Time) string {
	return fmt.Sprintf("rev-%d", takenAt.UnixNano())
}

func (m *RodManager) readSnapshotPayload(page *rod.Page, maxText int, maxElements int) (snapshotPayload, error) {
	raw, err := withRodResult(func() (string, error) {
		result, evalErr := page.Evaluate(rod.Eval(`
() => {
  const normalize = (value, max) => (value || "").replace(/\s+/g, " ").trim().slice(0, max);
  const selectorHint = (node) => {
    if (!node) return "";
    if (node.id && window.CSS && CSS.escape) return "#" + CSS.escape(node.id);
    if (node.id) return "#" + node.id;

    const parts = [];
    let current = node;
    while (current && current.nodeType === Node.ELEMENT_NODE && parts.length < 6) {
      let part = current.tagName.toLowerCase();
      if (current.getAttribute("name")) {
        part += '[name="' + String(current.getAttribute("name")).replace(/"/g, '\\"') + '"]';
      } else if (current.getAttribute("aria-label")) {
        part += '[aria-label="' + String(current.getAttribute("aria-label")).replace(/"/g, '\\"') + '"]';
      } else {
        const siblings = Array.from(current.parentElement ? current.parentElement.children : []).filter((child) => child.tagName === current.tagName);
        if (siblings.length > 1) {
          part += ':nth-of-type(' + (siblings.indexOf(current) + 1) + ')';
        }
      }
      parts.unshift(part);
      if (current.id) break;
      current = current.parentElement;
    }
    return parts.join(' > ');
  };
  const elements = Array.from(document.querySelectorAll('a,button,input,textarea,select,[role="button"],[role="link"]'))
    .filter((node) => {
      const rect = node.getBoundingClientRect();
      return rect.width > 0 || rect.height > 0;
    })
    .slice(0, ` + fmt.Sprintf("%d", maxElements) + `)
    .map((node, index) => ({
      id: "e" + (index + 1),
      role: node.getAttribute("role") || node.tagName.toLowerCase(),
      name: normalize(node.getAttribute("aria-label") || node.innerText || node.value || node.placeholder, 120),
      text: normalize(node.innerText || "", 120),
      selector_hint: selectorHint(node),
      editable: !!(node.matches('input,textarea,select,[contenteditable="true"]')),
      disabled: !!node.disabled
    }));
  const headings = Array.from(document.querySelectorAll('h1,h2,h3,h4'))
    .map((node) => ({
      level: Number(String(node.tagName || 'H1').replace(/^H/i, "")) || 1,
      text: normalize(node.innerText || "", 160)
    }))
    .filter((heading) => heading.text)
    .slice(0, 12);
  return JSON.stringify({
    url: location.href,
    title: document.title,
    text: normalize(document.body ? document.body.innerText : "", ` + fmt.Sprintf("%d", maxText) + `),
    elements,
    headings
  });
}`))
		if evalErr != nil {
			return "", evalErr
		}
		return result.Value.String(), nil
	})
	if err != nil {
		return snapshotPayload{}, err
	}
	var payload snapshotPayload
	if unmarshalErr := json.Unmarshal([]byte(raw), &payload); unmarshalErr != nil {
		return snapshotPayload{}, fmt.Errorf("decode browser snapshot: %w", unmarshalErr)
	}
	return payload, nil
}

func (m *RodManager) persistScreenshot(ctx context.Context, sessionID types.SessionID, pageID string, image []byte) (string, error) {
	if m.config.ArtifactStore == nil {
		return "", nil
	}
	ref, err := m.config.ArtifactStore.PutArtifact(ctx, storage.ArtifactPutRequest{
		Namespace:   storage.NamespaceBrowserScreenshots,
		Filename:    "screenshot.png",
		SessionID:   string(sessionID),
		PageID:      pageID,
		ContentType: "image/png",
	}, image)
	if err != nil {
		return "", err
	}
	return ref.URL, nil
}
