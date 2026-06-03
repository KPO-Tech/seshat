package askuser

import (
	"fmt"
	"strings"
)

const (
	minQuestionOptions = 2
	maxQuestionOptions = 10
)

// QuestionOption represents an option for a question.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Preview     string `json:"preview,omitempty"`
}

// Question represents a question to ask the user.
type Question struct {
	Question    string           `json:"question"`
	Header      string           `json:"header"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multiSelect,omitempty"`
}

// Input represents the tool input.
type Input struct {
	Questions   []Question            `json:"questions"`
	Answers     map[string]string     `json:"answers,omitempty"`
	Annotations map[string]Annotation `json:"annotations,omitempty"`
}

// Output represents the tool output.
type Output struct {
	Questions   []Question            `json:"questions"`
	Answers     map[string]string     `json:"answers"`
	Annotations map[string]Annotation `json:"annotations,omitempty"`
}

// Annotation contains additional info about a user's answer.
type Annotation struct {
	Preview string `json:"preview,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// Validate validates the input.
func (i *Input) Validate() error {
	if len(i.Questions) == 0 {
		return fmt.Errorf("at least one question is required")
	}
	if len(i.Questions) > 4 {
		return fmt.Errorf("maximum 4 questions allowed")
	}

	seenQuestions := make(map[string]bool)
	for _, q := range i.Questions {
		if strings.TrimSpace(q.Question) == "" {
			return fmt.Errorf("question text is required")
		}
		if seenQuestions[q.Question] {
			return fmt.Errorf("duplicate question: %s", q.Question)
		}
		seenQuestions[q.Question] = true

		if len(q.Options) < minQuestionOptions {
			return fmt.Errorf("question %s needs at least %d options", q.Question, minQuestionOptions)
		}
		if len(q.Options) > maxQuestionOptions {
			return fmt.Errorf("question %s has maximum %d options", q.Question, maxQuestionOptions)
		}

		seenLabels := make(map[string]bool)
		for _, opt := range q.Options {
			if strings.TrimSpace(opt.Label) == "" {
				return fmt.Errorf("option label is required")
			}
			if seenLabels[opt.Label] {
				return fmt.Errorf("duplicate option label: %s", opt.Label)
			}
			seenLabels[opt.Label] = true
		}
	}

	return nil
}

// FormatForDisplay formats the questions for display to user.
func (i *Input) FormatForDisplay() string {
	var builder strings.Builder
	builder.WriteString("Please answer the following question(s):\n\n")

	for idx, q := range i.Questions {
		builder.WriteString(fmt.Sprintf("%d. [%s] %s\n", idx+1, q.Header, q.Question))
		for optIdx, opt := range q.Options {
			builder.WriteString(fmt.Sprintf("   %d) %s", optIdx+1, opt.Label))
			if opt.Description != "" {
				builder.WriteString(fmt.Sprintf(" - %s", opt.Description))
			}
			builder.WriteString("\n")
		}
		if q.MultiSelect {
			builder.WriteString("   (Select multiple - enter numbers separated by comma)\n")
		} else {
			builder.WriteString("   (Enter number)\n")
		}
		builder.WriteString("\n")
	}

	builder.WriteString("Or enter 'other' to provide custom input.\n")
	return builder.String()
}

// GetOptionByIndex returns the option for a given question and index.
func (q *Question) GetOptionByIndex(index int) *QuestionOption {
	if index < 0 || index >= len(q.Options) {
		return nil
	}
	return &q.Options[index]
}

// GetOptionByLabel returns the option for a given question and label.
func (q *Question) GetOptionByLabel(label string) *QuestionOption {
	for i, opt := range q.Options {
		if opt.Label == label {
			return &q.Options[i]
		}
	}
	return nil
}
