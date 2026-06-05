package components

import "time"

const (
	doubleClickThreshold = 450 * time.Millisecond
	clickSlop            = 1
)

type mouseSelection struct {
	dragging bool
	moved    bool
	active   bool

	startLine int
	startCol  int
	endLine   int
	endCol    int

	lastClickAt    time.Time
	lastClickLine  int
	lastClickCol   int
	lastClickCount int
}

func (s *mouseSelection) begin(line, col int, now time.Time) int {
	count := 1
	if !s.lastClickAt.IsZero() && now.Sub(s.lastClickAt) <= doubleClickThreshold && absInt(line-s.lastClickLine) == 0 && absInt(col-s.lastClickCol) <= clickSlop {
		count = s.lastClickCount + 1
		if count > 3 {
			count = 1
		}
	}
	s.lastClickAt = now
	s.lastClickLine = line
	s.lastClickCol = col
	s.lastClickCount = count

	s.dragging = count == 1
	s.moved = false
	s.active = false
	s.startLine = line
	s.startCol = col
	s.endLine = line
	s.endCol = col
	return count
}

func (s *mouseSelection) update(line, col int) {
	if !s.dragging {
		return
	}
	s.endLine = line
	s.endCol = col
	if line != s.startLine || col != s.startCol {
		s.moved = true
	}
}

func (s *mouseSelection) finish() bool {
	wasMoved := s.moved
	s.dragging = false
	s.moved = false
	if wasMoved {
		s.active = true
	}
	return wasMoved
}

func (s *mouseSelection) clear() {
	s.dragging = false
	s.moved = false
	s.active = false
	s.startLine = 0
	s.startCol = 0
	s.endLine = 0
	s.endCol = 0
}

func (s *mouseSelection) setRange(startLine, startCol, endLine, endCol int) {
	s.dragging = false
	s.moved = false
	s.active = true
	s.startLine = startLine
	s.startCol = startCol
	s.endLine = endLine
	s.endCol = endCol
}

func (s *mouseSelection) hasSelection() bool {
	return s.active || (s.dragging && s.moved)
}

func (s *mouseSelection) rangeOrInvalid() (int, int, int, int) {
	if !s.hasSelection() {
		return -1, -1, -1, -1
	}
	startLn, startCo := s.startLine, s.startCol
	endLn, endCo := s.endLine, s.endCol
	if endLn < startLn || (endLn == startLn && endCo < startCo) {
		startLn, endLn = endLn, startLn
		startCo, endCo = endCo, startCo
	}
	return startLn, startCo, endLn, endCo
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
