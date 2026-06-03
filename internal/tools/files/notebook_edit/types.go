package notebookedit

import "time"

// Input defines the parameters for notebook editing operations.
type Input struct {
	NotebookPath string   `json:"notebook_path"`
	CellID       string   `json:"cell_id,omitempty"`
	NewSource    string   `json:"new_source"`
	CellType     string   `json:"cell_type,omitempty"`
	EditMode     EditMode `json:"edit_mode,omitempty"`
}

// EditMode defines the type of edit operation to perform on a notebook cell.
type EditMode string

const (
	EditModeReplace EditMode = "replace"
	EditModeInsert  EditMode = "insert"
	EditModeDelete  EditMode = "delete"
)

// Validate validates the input parameters for notebook editing.
// It checks that required fields are present and values are valid.
func (i *Input) Validate() error {
	if i.NotebookPath == "" {
		return &ValidationError{Code: 1, Message: "notebook_path is required"}
	}
	if i.EditMode == "" {
		i.EditMode = EditModeReplace
	}
	if (i.EditMode == EditModeReplace || i.EditMode == EditModeInsert) && i.NewSource == "" {
		return &ValidationError{Code: 3, Message: "new_source is required"}
	}
	if i.EditMode != EditModeReplace && i.EditMode != EditModeInsert && i.EditMode != EditModeDelete {
		return &ValidationError{Code: 4, Message: "edit_mode must be replace, insert, or delete"}
	}
	if i.EditMode == EditModeInsert && i.CellType == "" {
		return &ValidationError{Code: 5, Message: "cell_type is required when using edit_mode=insert"}
	}
	return nil
}

// Output defines the result of a notebook editing operation.
type Output struct {
	NewSource    string `json:"new_source"`
	CellID       string `json:"cell_id,omitempty"`
	CellType     string `json:"cell_type"`
	Language     string `json:"language"`
	EditMode     string `json:"edit_mode"`
	Error        string `json:"error,omitempty"`
	NotebookPath string `json:"notebook_path"`
	OriginalFile string `json:"original_file,omitempty"`
	UpdatedFile  string `json:"updated_file,omitempty"`
}

// ReadState represents the cached state of a read file operation.
// Used for read-before-edit validation.
type ReadState struct {
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ValidationError represents an error during input validation.
type ValidationError struct {
	Code    int    `json:"errorCode"`
	Message string `json:"message"`
}

// Error returns the validation error message.
func (e *ValidationError) Error() string {
	return e.Message
}
