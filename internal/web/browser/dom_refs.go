package browser

import (
	"fmt"
	"time"
)

func (p *pageState) invalidateDOMRefs() {
	if p == nil {
		return
	}
	p.domRevision = ""
	p.domRefs = map[string]domRef{}
	p.lastSnapshotText = ""
	p.lastSnapshotHeadings = nil
}

func (p *pageState) rebuildDOMRefs(elements []ElementInfo, revision string, text string, takenAt time.Time) {
	if p == nil {
		return
	}
	refs := make(map[string]domRef, len(elements))
	for _, element := range elements {
		refs[element.ID] = domRef{
			SelectorHint: element.SelectorHint,
			Role:         element.Role,
			Editable:     element.Editable,
		}
	}
	p.domRevision = revision
	p.domRefs = refs
	p.lastSnapshotAt = takenAt
	p.lastSnapshotText = text
}

func (p *pageState) selectorForAction(elementID string, revision string, requireEditable bool) (domRef, error) {
	if p == nil {
		return domRef{}, fmt.Errorf("browser page state missing")
	}
	if revision == "" {
		return domRef{}, fmt.Errorf("revision is required; run browser_snapshot again before acting")
	}
	if p.domRevision == "" || p.domRevision != revision {
		return domRef{}, fmt.Errorf("browser snapshot revision %q is stale; run browser_snapshot again", revision)
	}
	ref, ok := p.domRefs[elementID]
	if !ok || ref.SelectorHint == "" {
		return domRef{}, fmt.Errorf("element %q is not available in revision %q", elementID, revision)
	}
	if requireEditable && !ref.Editable {
		return domRef{}, fmt.Errorf("element %q is not editable in revision %q", elementID, revision)
	}
	return ref, nil
}
