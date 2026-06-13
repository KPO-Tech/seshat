package planreview

import (
	"sort"
	"strings"
)

// Submission is the frontend-facing plan review payload emitted when the
// runtime submits a plan for user review.
type Submission struct {
	SessionID string
	PlanID    string
	Slug      string
	Filename  string
	Status    string
	Version   int
	Content   string
}

// LineComment is feedback attached to a single 1-indexed source line.
type LineComment struct {
	Line    int
	Comment string
}

// Review captures the user's review decision and feedback.
type Review struct {
	Submission    Submission
	Approved      bool
	GlobalComment string
	LineComments  []LineComment
}

// HasFeedback reports whether the review includes any non-empty change request
// feedback.
func (r Review) HasFeedback() bool {
	if strings.TrimSpace(r.GlobalComment) != "" {
		return true
	}
	for _, comment := range r.LineComments {
		if strings.TrimSpace(comment.Comment) != "" {
			return true
		}
	}
	return false
}

// SortedLineComments returns a sorted copy of the attached line comments.
func (r Review) SortedLineComments() []LineComment {
	out := append([]LineComment(nil), r.LineComments...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Line == out[j].Line {
			return out[i].Comment < out[j].Comment
		}
		return out[i].Line < out[j].Line
	})
	return out
}
