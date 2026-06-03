package lspClient

import (
	"context"
	"time"
)

// Position represents a position in a text document
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range represents a range in a text document
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location in a text document
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// LocationLink represents a location link
type LocationLink struct {
	OriginURI   string `json:"originUri,omitempty"`
	OriginRange Range  `json:"originSelectionRange,omitempty"`
	TargetURI   string `json:"targetUri"`
	TargetRange Range  `json:"targetSelectionRange"`
}

// TextDocumentIdentifier identifies a text document
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// VersionedTextDocumentIdentifier identifies a text document with version
type VersionedTextDocumentIdentifier struct {
	TextDocumentIdentifier
	Version int `json:"version"`
}

// TextDocumentItem identifies a text document
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// TextDocumentPositionParams params for text document position requests
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// ReferenceParams params for reference requests
type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

// ReferenceContext context for reference requests
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// DocumentSymbolParams params for document symbol requests
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// WorkspaceSymbolParams params for workspace symbol requests
type WorkspaceSymbolParams struct {
	Query string `json:"query"`
}

// InitializeParams params for initialize request
type InitializeParams struct {
	ProcessID        int64              `json:"processId"`
	RootURI          string             `json:"rootUri,omitempty"`
	RootPath         string             `json:"rootPath,omitempty"`
	ClientInfo       ClientInfo         `json:"clientInfo"`
	Capabilities     ClientCapabilities `json:"capabilities"`
	WorkspaceFolders []WorkspaceFolder  `json:"workspaceFolders"`
	Locale           string             `json:"locale,omitempty"`
}

// ClientInfo client information
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities client capabilities
type ClientCapabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	Workspace    *WorkspaceClientCapabilities   `json:"workspace,omitempty"`
	Window       *WindowClientCapabilities      `json:"window,omitempty"`
}

// TextDocumentClientCapabilities text document capabilities
type TextDocumentClientCapabilities struct {
	Synchronization  *SynchronizationCapabilities  `json:"synchronization,omitempty"`
	Completion       *CompletionCapabilities       `json:"completion,omitempty"`
	Hover            *HoverCapabilities            `json:"hover,omitempty"`
	References       *ReferenceCapabilities        `json:"references,omitempty"`
	Definition       *DefinitionCapabilities       `json:"definition,omitempty"`
	TypeDefinition   *TypeDefinitionCapabilities   `json:"typeDefinition,omitempty"`
	Implementation   *ImplementationCapabilities   `json:"implementation,omitempty"`
	DocumentSymbol   *DocumentSymbolCapabilities   `json:"documentSymbol,omitempty"`
	CodeAction       *CodeActionCapabilities       `json:"codeAction,omitempty"`
	CodeLens         *CodeLensCapabilities         `json:"codeLens,omitempty"`
	Formatting       *FormattingCapabilities       `json:"formatting,omitempty"`
	RangeFormatting  *RangeFormattingCapabilities  `json:"rangeFormatting,omitempty"`
	OnTypeFormatting *OnTypeFormattingCapabilities `json:"onTypeFormatting,omitempty"`
	Rename           *RenameCapabilities           `json:"rename,omitempty"`
	SelectionRange   *SelectionRangeCapabilities   `json:"selectionRange,omitempty"`
}

// SynchronizationCapabilities synchronization capabilities
type SynchronizationCapabilities struct {
	WillSave          bool `json:"willSave,omitempty"`
	DidSave           bool `json:"didSave,omitempty"`
	WillSaveWaitUntil bool `json:"willSaveWaitUntil,omitempty"`
}

// CompletionCapabilities completion capabilities
type CompletionCapabilities struct {
	CompletionItem     *CompletionItemCapabilities     `json:"completionItem,omitempty"`
	CompletionItemKind *CompletionItemKindCapabilities `json:"completionItemKind,omitempty"`
	ContextSupport     bool                            `json:"contextSupport,omitempty"`
	CompletionList     *CompletionListCapabilities     `json:"completionList,omitempty"`
}

// CompletionItemCapabilities completion item capabilities
type CompletionItemCapabilities struct {
	SnippetSupport          bool     `json:"snippetSupport,omitempty"`
	CommitCharactersSupport bool     `json:"commitCharactersSupport,omitempty"`
	DocumentationFormat     []string `json:"documentationFormat,omitempty"`
	DeprecatedSupport       bool     `json:"deprecatedSupport,omitempty"`
	PreselectSupport        bool     `json:"preselectSupport,omitempty"`
}

// CompletionItemKindCapabilities completion item kind capabilities
type CompletionItemKindCapabilities struct {
	ValueSet []int `json:"valueSet,omitempty"`
}

// CompletionListCapabilities completion list capabilities
type CompletionListCapabilities struct {
	ItemDefaults []string `json:"itemDefaults,omitempty"`
}

// HoverCapabilities hover capabilities
type HoverCapabilities struct {
	ContentFormat []string `json:"contentFormat,omitempty"`
}

// ReferenceCapabilities reference capabilities
type ReferenceCapabilities struct {
	ContextSupport bool `json:"contextSupport,omitempty"`
}

// DefinitionCapabilities definition capabilities
type DefinitionCapabilities struct {
	LinkSupport bool `json:"linkSupport,omitempty"`
}

// TypeDefinitionCapabilities type definition capabilities
type TypeDefinitionCapabilities struct {
	LinkSupport bool `json:"linkSupport,omitempty"`
}

// ImplementationCapabilities implementation capabilities
type ImplementationCapabilities struct {
	LinkSupport bool `json:"linkSupport,omitempty"`
}

// DocumentSymbolCapabilities document symbol capabilities
type DocumentSymbolCapabilities struct {
	HierarchicalSymbolSupport bool `json:"hierarchicalSymbolSupport,omitempty"`
	LabelSupport              bool `json:"labelSupport,omitempty"`
}

// CodeActionCapabilities code action capabilities
type CodeActionCapabilities struct {
	CodeActionLiteralSupport *CodeActionLiteralSupportCapabilities `json:"codeActionLiteralSupport,omitempty"`
	IsDynamicRegistration    bool                                  `json:"isDynamicRegistration,omitempty"`
}

// CodeActionLiteralSupportCapabilities code action literal support capabilities
type CodeActionLiteralSupportCapabilities struct {
	CodeActionKind *CodeActionKindCapabilities `json:"codeActionKind,omitempty"`
}

// CodeActionKindCapabilities code action kind capabilities
type CodeActionKindCapabilities struct {
	ValueSet []string `json:"valueSet,omitempty"`
}

// CodeLensCapabilities code lens capabilities
type CodeLensCapabilities struct {
	IsDynamicRegistration bool `json:"isDynamicRegistration,omitempty"`
}

// FormattingCapabilities formatting capabilities
type FormattingCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// RangeFormattingCapabilities range formatting capabilities
type RangeFormattingCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// OnTypeFormattingCapabilities on type formatting capabilities
type OnTypeFormattingCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// RenameCapabilities rename capabilities
type RenameCapabilities struct {
	PrepareSupport                bool   `json:"prepareSupport,omitempty"`
	PrepareSupportDefaultBehavior string `json:"prepareSupportDefaultBehavior,omitempty"`
	HonorsChangeAnnotations       bool   `json:"honorsChangeAnnotations,omitempty"`
}

// SelectionRangeCapabilities selection range capabilities
type SelectionRangeCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// WorkspaceClientCapabilities workspace capabilities
type WorkspaceClientCapabilities struct {
	ApplyEdit        bool                       `json:"applyEdit,omitempty"`
	WorkspaceFolders bool                       `json:"workspaceFolders,omitempty"`
	WorkspaceEdit    *WorkspaceEditCapabilities `json:"workspaceEdit,omitempty"`
	FileOperations   *FileOperationCapabilities `json:"fileOperations,omitempty"`
	Symbol           *SymbolCapabilities        `json:"symbol,omitempty"`
}

// WorkspaceEditCapabilities workspace edit capabilities
type WorkspaceEditCapabilities struct {
	DocumentChanges    bool     `json:"documentChanges,omitempty"`
	ResourceOperations []string `json:"resourceOperations,omitempty"`
	FailureHandling    string   `json:"failureHandling,omitempty"`
}

// FileOperationCapabilities file operation capabilities
type FileOperationCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
	WillRename          bool `json:"willRename,omitempty"`
}

// SymbolCapabilities symbol capabilities
type SymbolCapabilities struct {
	DynamicRegistration bool                    `json:"dynamicRegistration,omitempty"`
	SymbolKind          *SymbolKindCapabilities `json:"symbolKind,omitempty"`
}

// SymbolKindCapabilities symbol kind capabilities
type SymbolKindCapabilities struct {
	ValueSet []int `json:"valueSet,omitempty"`
}

// WindowClientCapabilities window capabilities
type WindowClientCapabilities struct {
	WorkDoneProgress bool `json:"workDoneProgress,omitempty"`
}

// InitializeResult result of initialize request
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   ServerInfo         `json:"serverInfo,omitempty"`
}

// ServerInfo server information
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapabilities server capabilities
type ServerCapabilities struct {
	TextDocumentSync                 interface{}           `json:"textDocumentSync,omitempty"`
	HoverProvider                    bool                  `json:"hoverProvider,omitempty"`
	CompletionProvider               *CompletionOptions    `json:"completionProvider,omitempty"`
	SignatureHelpProvider            *SignatureHelpOptions `json:"signatureHelpProvider,omitempty"`
	DefinitionProvider               bool                  `json:"definitionProvider,omitempty"`
	TypeDefinitionProvider           interface{}           `json:"typeDefinitionProvider,omitempty"`
	ImplementationProvider           interface{}           `json:"implementationProvider,omitempty"`
	ReferencesProvider               bool                  `json:"referencesProvider,omitempty"`
	DocumentHighlightProvider        bool                  `json:"documentHighlightProvider,omitempty"`
	DocumentSymbolProvider           bool                  `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider          interface{}           `json:"workspaceSymbolProvider,omitempty"`
	CodeActionProvider               bool                  `json:"codeActionProvider,omitempty"`
	CodeLensProvider                 *CodeLensOptions      `json:"codeLensProvider,omitempty"`
	DocumentFormattingProvider       bool                  `json:"documentFormattingProvider,omitempty"`
	DocumentRangeFormattingProvider  bool                  `json:"documentRangeFormattingProvider,omitempty"`
	DocumentOnTypeFormattingProvider interface{}           `json:"documentOnTypeFormattingProvider,omitempty"`
	RenameProvider                   interface{}           `json:"renameProvider,omitempty"`
	DocumentLinkProvider             *DocumentLinkOptions  `json:"documentLinkProvider,omitempty"`
	ColorProvider                    interface{}           `json:"colorProvider,omitempty"`
	FoldingRangeProvider             interface{}           `json:"foldingRangeProvider,omitempty"`
	SelectionRangeProvider           interface{}           `json:"selectionRangeProvider,omitempty"`
	LinkedEditingRangeProvider       interface{}           `json:"linkedEditingRangeProvider,omitempty"`
	CallHierarchyProvider            bool                  `json:"callHierarchyProvider,omitempty"`
	SemanticTokensProvider           interface{}           `json:"semanticTokensProvider,omitempty"`
	MonikerProvider                  bool                  `json:"monikerProvider,omitempty"`
}

// CompletionOptions completion options
type CompletionOptions struct {
	ResolveProvider   bool     `json:"resolveProvider,omitempty"`
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// SignatureHelpOptions signature help options
type SignatureHelpOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

// CodeLensOptions code lens options
type CodeLensOptions struct {
	ResolveProvider bool `json:"resolveProvider,omitempty"`
}

// DocumentLinkOptions document link options
type DocumentLinkOptions struct {
	ResolveProvider bool `json:"resolveProvider,omitempty"`
}

// WorkspaceFolder represents a workspace folder
type WorkspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

// Hover represents hover information
type Hover struct {
	Contents interface{} `json:"contents"` // MarkupContent | string | MarkedString[]
	Range    *Range      `json:"range,omitempty"`
}

// DocumentSymbol represents a document symbol
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolInformation represents symbol information
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// CallHierarchyItem represents a call hierarchy item
type CallHierarchyItem struct {
	Name           string `json:"name"`
	Kind           int    `json:"kind"`
	URI            string `json:"uri"`
	Range          Range  `json:"range"`
	SelectionRange Range  `json:"selectionRange"`
	Detail         string `json:"detail,omitempty"`
}

// CallHierarchyIncomingCall represents an incoming call
type CallHierarchyIncomingCall struct {
	From       CallHierarchyItem `json:"from"`
	FromRanges []Range           `json:"fromRanges"`
}

// CallHierarchyOutgoingCall represents an outgoing call
type CallHierarchyOutgoingCall struct {
	To         CallHierarchyItem `json:"to"`
	FromRanges []Range           `json:"fromRanges"`
}

// CallHierarchyIncomingCallsParams params for incoming calls
type CallHierarchyIncomingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

// CallHierarchyOutgoingCallsParams params for outgoing calls
type CallHierarchyOutgoingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

// DidOpenTextDocumentParams params for didOpen notification
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams params for didChange notification
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// TextDocumentContentChangeEvent represents a text document change
type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

// DidSaveTextDocumentParams params for didSave notification
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

// DidCloseTextDocumentParams params for didClose notification
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// URIFromPath converts a file path to a file URI
func URIFromPath(path string) string {
	// Simple implementation - could be enhanced for Windows
	return "file://" + path
}

// PathFromURI converts a file URI to a path
func PathFromURI(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}

// DefaultTimeout is the default request timeout
const DefaultTimeout = 30 * time.Second

// RequestWithTimeout sends a request with a timeout
func (c *Client) RequestWithTimeout(ctx context.Context, method string, params interface{}, result interface{}, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.Request(ctx, method, params, result)
}

// ServerCapabilities helpers

// IsDefinitionProvider returns true if the server supports definition
func (s *ServerCapabilities) IsDefinitionProvider() bool {
	return s.DefinitionProvider
}

// IsReferencesProvider returns true if the server supports references
func (s *ServerCapabilities) IsReferencesProvider() bool {
	return s.ReferencesProvider
}

// IsHoverProvider returns true if the server supports hover
func (s *ServerCapabilities) IsHoverProvider() bool {
	return s.HoverProvider
}

// IsDocumentSymbolProvider returns true if the server supports document symbols
func (s *ServerCapabilities) IsDocumentSymbolProvider() bool {
	return s.DocumentSymbolProvider
}

// IsWorkspaceSymbolProvider returns true if the server supports workspace symbols
func (s *ServerCapabilities) IsWorkspaceSymbolProvider() bool {
	return s.WorkspaceSymbolProvider != nil
}

// IsImplementationProvider returns true if the server supports implementation
func (s *ServerCapabilities) IsImplementationProvider() bool {
	return s.ImplementationProvider != nil
}

// IsCallHierarchyProvider returns true if the server supports call hierarchy
func (s *ServerCapabilities) IsCallHierarchyProvider() bool {
	return s.CallHierarchyProvider
}

// SymbolKind constants (from LSP spec)
const (
	SymbolKindFile          = 1
	SymbolKindModule        = 2
	SymbolKindNamespace     = 3
	SymbolKindPackage       = 4
	SymbolKindClass         = 5
	SymbolKindMethod        = 6
	SymbolKindProperty      = 7
	SymbolKindField         = 8
	SymbolKindConstructor   = 9
	SymbolKindEnum          = 10
	SymbolKindInterface     = 11
	SymbolKindConstant      = 12
	SymbolKindString        = 13
	SymbolKindNumber        = 14
	SymbolKindBoolean       = 15
	SymbolKindArray         = 16
	SymbolKindObject        = 17
	SymbolKindKey           = 18
	SymbolKindNull          = 19
	SymbolKindEnumMember    = 20
	SymbolKindStruct        = 21
	SymbolKindEvent         = 22
	SymbolKindOperator      = 23
	SymbolKindTypeParameter = 24
)

// TextDocumentSyncKind constants
const (
	TextDocumentSyncNone        = 0
	TextDocumentSyncFull        = 1
	TextDocumentSyncIncremental = 2
)
