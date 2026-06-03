package browser

import "fmt"

func (s *sessionState) allocatePageID() string {
	s.nextPageSeq++
	return fmt.Sprintf("page-%d", s.nextPageSeq)
}

func (s *sessionState) addPage(state *pageState) {
	if state == nil {
		return
	}
	s.pages[state.info.ID] = state
	if state.targetID != "" {
		if s.pageTargets == nil {
			s.pageTargets = make(map[string]string)
		}
		s.pageTargets[state.targetID] = state.info.ID
	}
	s.pageOrder = append(s.pageOrder, state.info.ID)
	s.setActivePage(state.info.ID)
}

func (s *sessionState) removePage(pageID string) *pageState {
	page, ok := s.pages[pageID]
	if !ok {
		return nil
	}
	if page.watchCancel != nil {
		page.watchCancel()
		page.watchCancel = nil
	}
	if page.targetID != "" {
		delete(s.pageTargets, page.targetID)
	}
	delete(s.pages, pageID)
	filtered := s.pageOrder[:0]
	for _, existing := range s.pageOrder {
		if existing != pageID {
			filtered = append(filtered, existing)
		}
	}
	s.pageOrder = filtered
	if s.activePageID == pageID {
		s.activePageID = ""
		if replacement := s.firstAvailablePageID(); replacement != "" {
			s.setActivePage(replacement)
		}
	}
	return page
}

func (s *sessionState) setActivePage(pageID string) {
	s.activePageID = pageID
	for _, id := range s.pageOrder {
		if page, ok := s.pages[id]; ok {
			page.info.Active = id == pageID
		}
	}
}

func (s *sessionState) firstAvailablePageID() string {
	for _, id := range s.pageOrder {
		if _, ok := s.pages[id]; ok {
			return id
		}
	}
	return ""
}

func (s *sessionState) orderedPageInfos() []PageInfo {
	pages := make([]PageInfo, 0, len(s.pageOrder))
	for _, id := range s.pageOrder {
		if page, ok := s.pages[id]; ok {
			pages = append(pages, page.info)
		}
	}
	return pages
}
