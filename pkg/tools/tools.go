// Package tools exposes the seshat agent's tool contract publicly, so an
// external consumer (e.g. seshat-ai) can implement a custom tool and
// register it via sdk.Client.RegisterTool/RegisterTools.
//
// Before this package existed, sdk.Tool was a public alias to
// internal/tools/contract.Tool, but that interface's own method set
// referenced several types (CallInput, Definition, ToolUseContext,
// CanUseToolFn, the JSON schema builder) that had no public path - an
// external module could reference the interface *name* via the alias, but
// couldn't actually write a method signature that implements it, since Go's
// internal/ package rule blocks importing internal/tools/contract or
// internal/types directly from outside this module. This package closes
// that gap by re-exporting exactly what's needed to write and register a
// real tool from application code, mirroring the existing internal
// implementations (see internal/tools/special/rag/rag_search.go for the
// canonical example this package's aliases were derived from).
package tools

import (
	internalcontract "github.com/KPO-Tech/seshat/internal/tools/contract"
	internalschema "github.com/KPO-Tech/seshat/internal/tools/schema"
	internaltypes "github.com/KPO-Tech/seshat/internal/types"
)

type (
	// Tool is the interface a custom tool must implement. Identical to
	// sdk.Tool (both alias internal/tools/contract.Tool) - implementing this
	// satisfies sdk.Tool too, so the result can be passed directly to
	// sdk.Client.RegisterTool without any adapter.
	Tool = internalcontract.Tool

	// Toolset is a named group of tools resolved dynamically at the start of
	// each loop iteration, for callers whose available tools depend on
	// runtime state rather than being fixed at registration time.
	Toolset = internalcontract.Toolset

	// Definition defines a tool's metadata and schema
	Definition = internalcontract.Definition
	// CallInput represents the input to a tool call
	CallInput = internalcontract.CallInput
	// CallResult represents the result of a tool call
	CallResult = internalcontract.CallResult
	// ContentType represents the type of tool result
	ContentType = internalcontract.ContentType
	// ResultMetadata contains metadata about a tool result
	ResultMetadata = internalcontract.ResultMetadata
	// ToolUseContext provides context for tool execution.
	ToolUseContext = internalcontract.ToolUseContext

	// ContextModifier mutates the tool runtime context after a successful call.
	ContextModifier = internalcontract.ContextModifier

	// JSONSchema describes a tool's InputSchema. Build one with FromMap
	// rather than constructing it by hand.
	JSONSchema = internalschema.JSONSchema

	// CanUseToolFn is the permission-check function threaded through
	// ToolUseContext.CanUseTool and Tool.Call's permissionCheck parameter.
	CanUseToolFn = internaltypes.CanUseToolFn

	// PermissionResult is CheckPermissions' return type - build one with
	// AllowWithInput or Passthrough rather than constructing it by hand.
	PermissionResult = internaltypes.PermissionResult
)

const (
	ContentTypeText   = internalcontract.ContentTypeText
	ContentTypeJSON   = internalcontract.ContentTypeJSON
	ContentTypeBinary = internalcontract.ContentTypeBinary
	ContentTypeStream = internalcontract.ContentTypeStream
	ContentTypeMixed  = internalcontract.ContentTypeMixed
)

// NewToolUseContext creates a new ToolUseContext - mainly useful in tests;
// production code normally receives one via CallInput.ToolContextValue().
func NewToolUseContext(sessionID internaltypes.SessionID, turnID internaltypes.TurnID, toolUseID string, permissionMode internaltypes.PermissionMode) ToolUseContext {
	return internalcontract.NewToolUseContext(sessionID, turnID, toolUseID, permissionMode)
}

// NewTextResult creates a plain-text CallResult.
func NewTextResult(content string) CallResult {
	return internalcontract.NewTextResult(content)
}

// NewJSONResult creates a CallResult whose Content defaults to data's JSON
// encoding - set .Content afterward for a human-readable summary instead,
// which is what most tools want (see internalcontract.NewJSONResult's own
// doc comment for why the default exists at all).
func NewJSONResult(data any) CallResult {
	return internalcontract.NewJSONResult(data)
}

// NewErrorResult creates an error CallResult.
func NewErrorResult(err error) CallResult {
	return internalcontract.NewErrorResult(err)
}

// FromMap builds a JSONSchema from a plain map literal - the standard way
// to write Definition.InputSchema (see any internal/tools/special/*
// implementation for the expected shape: {"type": "object", "properties":
// {...}, "required": [...]}).
func FromMap(schemaMap map[string]any) JSONSchema {
	return internalschema.FromMap(schemaMap)
}

// AllowWithInput creates an Allow PermissionResult with normalized input -
// the common case for a tool's CheckPermissions when it has nothing extra
// to enforce beyond the global permission pipeline's own checks.
func AllowWithInput(reason string, updatedInput map[string]any) PermissionResult {
	return internaltypes.AllowWithInput(reason, updatedInput)
}

// Passthrough delegates the permission decision entirely to the global
// pipeline - use this from CheckPermissions when a tool has no
// tool-specific rule of its own to apply.
func Passthrough(updatedInput map[string]any) PermissionResult {
	return internaltypes.Passthrough(updatedInput)
}
