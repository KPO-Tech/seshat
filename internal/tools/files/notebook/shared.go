// Package notebook implements notebook_edit, notebook_create, and notebook_write tools.
package notebook

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	read "github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
)

const (
	defaultKernel   = "python3"
	defaultLanguage = "python"
)

// CellSpec describes a cell for notebook_create and notebook_write.
type CellSpec struct {
	CellType string `json:"cell_type"` // "code" or "markdown"
	Source   string `json:"source"`
}

// validationError is a structured input validation error shared by all notebook tools.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

// buildNotebook assembles a valid nbformat 4.5 notebook from a kernel + cell list.
func buildNotebook(kernel, language string, specs []CellSpec) read.Notebook {
	nb := read.Notebook{
		Cells: make([]read.NotebookCell, 0, len(specs)),
		Metadata: read.NotebookMetadata{
			Kernelspec: &read.NotebookKernelspec{
				Name:     kernel,
				Display:  kernelDisplayName(kernel),
				Language: language,
			},
			LanguageInfo: &read.NotebookLanguageInfo{Name: language},
		},
		NBFormat:      4,
		NBFormatMinor: 5,
	}
	for _, s := range specs {
		nb.Cells = append(nb.Cells, buildCell(s.CellType, s.Source))
	}
	return nb
}

// buildCell creates a single notebook cell with a unique ID.
func buildCell(cellType, source string) read.NotebookCell {
	cell := read.NotebookCell{
		ID:       generateCellID(),
		CellType: cellType,
		Source:   []string{source},
		Metadata: json.RawMessage("{}"),
	}
	if cellType == "code" {
		cell.Outputs = []read.NotebookOutput{}
	}
	return cell
}

var globalCellSeq int64

// generateCellID returns a 12-char hex cell ID unique across concurrent calls.
func generateCellID() string {
	seq := atomic.AddInt64(&globalCellSeq, 1)
	return fmt.Sprintf("%012x", time.Now().UnixNano()+seq)
}

func kernelDisplayName(kernel string) string {
	switch kernel {
	case "python3":
		return "Python 3"
	case "python2":
		return "Python 2"
	case "ir":
		return "R"
	default:
		return kernel
	}
}

// emptyNotebookJSON returns a minimal valid nbformat 4 notebook.
// Used by notebook_edit insert mode when the target file does not exist yet.
func emptyNotebookJSON() []byte {
	return []byte(`{
 "cells": [],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3",
   "language": "python",
   "name": "python3"
  },
  "language_info": {
   "name": "python",
   "version": "3.8.0"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 5
}`)
}

// parseCellSpecs converts a raw []any (from JSON unmarshalling) to []CellSpec.
func parseCellSpecs(raw []any) []CellSpec {
	specs := make([]CellSpec, 0, len(raw))
	for _, rc := range raw {
		if m, ok := rc.(map[string]any); ok {
			cs := CellSpec{}
			if ct, ok := m["cell_type"].(string); ok {
				cs.CellType = ct
			}
			if src, ok := m["source"].(string); ok {
				cs.Source = src
			}
			specs = append(specs, cs)
		}
	}
	return specs
}
