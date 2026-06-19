// Package notebook implements all Jupyter notebook tools: file management
// (create, read, write, edit) and kernel execution (execute, run, kernel).
package notebook

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/tools/files/read"
)

const (
	defaultKernel   = "python3"
	defaultLanguage = "python"
)

// CellSpec describes a single cell for create/write/edit operations.
type CellSpec struct {
	CellType string `json:"cell_type"` // "code" or "markdown"
	Source   string `json:"source"`
}

// validationError carries a human-readable validation failure.
type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

// buildNotebook assembles a valid nbformat 4.5 notebook.
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

// generateCellID returns a 12-char hex cell ID unique within a process.
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

// emptyNotebookJSON returns a minimal valid nbformat 4.5 notebook JSON.
func emptyNotebookJSON() []byte {
	return []byte(`{
 "cells": [],
 "metadata": {
  "kernelspec": {"display_name": "Python 3", "language": "python", "name": "python3"},
  "language_info": {"name": "python", "version": "3.8.0"}
 },
 "nbformat": 4,
 "nbformat_minor": 5
}`)
}

// parseCellSpecs converts []any from JSON decode into []CellSpec.
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

// cellSource joins a []string source array into a single string.
func cellSource(cell read.NotebookCell) string {
	if len(cell.Source) == 1 {
		return cell.Source[0]
	}
	result := ""
	for _, s := range cell.Source {
		result += s
	}
	return result
}
