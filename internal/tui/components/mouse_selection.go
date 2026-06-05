package components

type mouseSelection struct {
	dragging bool
	moved    bool
	active   bool

	startLine int
	startCol  int
	endLine   int
	endCol    int
}

func (s *mouseSelection) begin(line, col int) {
	s.dragging = true
	s.moved = false
	s.startLine = line
	s.startCol = col
	s.endLine = line
	s.endCol = col
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

func (s *mouseSelection) hasSelection() bool {
	return s.active || (s.dragging && s.moved)
}

func (s *mouseSelection) clearDrag() {
	s.dragging = false
	s.moved = false
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
