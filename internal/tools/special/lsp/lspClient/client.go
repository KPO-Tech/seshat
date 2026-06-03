package lspClient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// RPCMessage represents a JSON-RPC message
type RPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// Client represents an LSP client connection
type Client struct {
	mu           sync.RWMutex
	nextID       int
	pending      map[int]chan *RPCMessage
	serverName   string
	capabilities ServerCapabilities

	stdin    io.Writer
	stdout   io.Reader
	readDone chan struct{}

	// Callbacks for notifications
	notifications map[string]NotificationHandler
	muNotif       sync.RWMutex
}

// NotificationHandler handles LSP notifications
type NotificationHandler func(params json.RawMessage)

// NewClient creates a new LSP client
func NewClient(serverName string) *Client {
	return &Client{
		serverName:    serverName,
		nextID:        1,
		pending:       make(map[int]chan *RPCMessage),
		notifications: make(map[string]NotificationHandler),
		readDone:      make(chan struct{}),
	}
}

// ConnectProcess connects to an LSP server via an existing process
func (c *Client) ConnectProcess(stdin io.Writer, stdout io.Reader) error {
	c.stdin = stdin
	c.stdout = stdout

	// Start message handling in background
	go c.handleMessages()

	return nil
}

// Initialize initializes the LSP server
func (c *Client) Initialize(ctx context.Context, rootPath string) (*InitializeResult, error) {
	params := InitializeParams{
		ProcessID:  int64(os.Getpid()),
		RootURI:    URIFromPath(rootPath),
		ClientInfo: ClientInfo{Name: "Nexus AI", Version: "1.0.0"},
		Capabilities: ClientCapabilities{
			TextDocument: TextDocumentClientCapabilities{
				Synchronization: &SynchronizationCapabilities{
					WillSave:          true,
					DidSave:           true,
					WillSaveWaitUntil: true,
				},
				Completion: &CompletionCapabilities{
					CompletionItem: &CompletionItemCapabilities{
						SnippetSupport: true,
					},
				},
				Hover: &HoverCapabilities{
					ContentFormat: []string{"markdown", "plaintext"},
				},
				References:     &ReferenceCapabilities{},
				Definition:     &DefinitionCapabilities{},
				TypeDefinition: &TypeDefinitionCapabilities{},
				Implementation: &ImplementationCapabilities{},
			},
			Workspace: &WorkspaceClientCapabilities{
				ApplyEdit:        true,
				WorkspaceFolders: true,
				Symbol:           &SymbolCapabilities{},
			},
		},
		WorkspaceFolders: []WorkspaceFolder{
			{Name: "workspace", URI: URIFromPath(rootPath)},
		},
	}

	var result InitializeResult
	err := c.Request(ctx, "initialize", params, &result)
	if err != nil {
		return nil, err
	}

	c.capabilities = result.Capabilities

	// Send initialized notification
	c.Notify(ctx, "initialized", nil)

	return &result, nil
}

// Shutdown sends the shutdown request
func (c *Client) Shutdown(ctx context.Context) error {
	var result interface{}
	err := c.Request(ctx, "shutdown", nil, &result)
	if err != nil {
		return err
	}
	c.Notify(ctx, "exit", nil)
	return nil
}

// Close closes the client connection
func (c *Client) Close() error {
	close(c.readDone)
	return nil
}

// TextDocumentDefinitions sends a textDocument/definition request
func (c *Client) TextDocumentDefinition(ctx context.Context, uri string, position Position) ([]Location, error) {
	var result interface{}
	err := c.Request(ctx, "textDocument/definition", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     position,
	}, &result)
	if err != nil {
		return nil, err
	}

	return locationsFromResult(result)
}

// TextDocumentReferences sends a textDocument/references request
func (c *Client) TextDocumentReferences(ctx context.Context, uri string, position Position) ([]Location, error) {
	var result interface{}
	err := c.Request(ctx, "textDocument/references", ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     position,
		},
		Context: ReferenceContext{IncludeDeclaration: true},
	}, &result)
	if err != nil {
		return nil, err
	}

	return locationsFromResult(result)
}

// TextDocumentHover sends a textDocument/hover request
func (c *Client) TextDocumentHover(ctx context.Context, uri string, position Position) (*Hover, error) {
	var result *Hover
	err := c.Request(ctx, "textDocument/hover", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     position,
	}, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// TextDocumentDocumentSymbol sends a textDocument/documentSymbol request
func (c *Client) TextDocumentDocumentSymbol(ctx context.Context, uri string) ([]DocumentSymbol, error) {
	var result interface{}
	err := c.Request(ctx, "textDocument/documentSymbol", DocumentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}, &result)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	// Can be either []DocumentSymbol or []SymbolInformation
	switch arr := result.(type) {
	case []interface{}:
		symbols := make([]DocumentSymbol, 0, len(arr))
		for _, item := range arr {
			if data, ok := item.(map[string]interface{}); ok {
				sym := documentSymbolFromMap(data)
				symbols = append(symbols, sym)
			}
		}
		return symbols, nil
	}

	return nil, nil
}

// WorkspaceSymbol sends a workspace/symbol request
func (c *Client) WorkspaceSymbol(ctx context.Context, query string) ([]SymbolInformation, error) {
	var result []SymbolInformation
	err := c.Request(ctx, "workspace/symbol", WorkspaceSymbolParams{
		Query: query,
	}, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// TextDocumentImplementation sends a textDocument/implementation request
func (c *Client) TextDocumentImplementation(ctx context.Context, uri string, position Position) ([]Location, error) {
	var result interface{}
	err := c.Request(ctx, "textDocument/implementation", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     position,
	}, &result)
	if err != nil {
		return nil, err
	}

	return locationsFromResult(result)
}

// TextDocumentPrepareCallHierarchy sends a textDocument/prepareCallHierarchy request
func (c *Client) TextDocumentPrepareCallHierarchy(ctx context.Context, uri string, position Position) ([]CallHierarchyItem, error) {
	var result []CallHierarchyItem
	err := c.Request(ctx, "textDocument/prepareCallHierarchy", TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     position,
	}, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CallHierarchyIncomingCalls sends a callHierarchy/incomingCalls request
func (c *Client) CallHierarchyIncomingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyIncomingCall, error) {
	var result []CallHierarchyIncomingCall
	err := c.Request(ctx, "callHierarchy/incomingCalls", CallHierarchyIncomingCallsParams{
		Item: item,
	}, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CallHierarchyOutgoingCalls sends a callHierarchy/outgoingCalls request
func (c *Client) CallHierarchyOutgoingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyOutgoingCall, error) {
	var result []CallHierarchyOutgoingCall
	err := c.Request(ctx, "callHierarchy/outgoingCalls", CallHierarchyOutgoingCallsParams{
		Item: item,
	}, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// TextDocumentOpen opens a document in the LSP server
func (c *Client) TextDocumentOpen(ctx context.Context, uri string, content string, languageID string) error {
	return c.Notify(ctx, "textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    1,
			Text:       content,
		},
	})
}

// TextDocumentChange sends changes to the LSP server
func (c *Client) TextDocumentChange(ctx context.Context, uri string, content string, version int) error {
	return c.Notify(ctx, "textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: TextDocumentIdentifier{URI: uri},
			Version:                version + 1,
		},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: content},
		},
	})
}

// TextDocumentSave saves the document
func (c *Client) TextDocumentSave(ctx context.Context, uri string, text string) error {
	return c.Notify(ctx, "textDocument/didSave", DidSaveTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Text:         &text,
	})
}

// TextDocumentClose closes a document
func (c *Client) TextDocumentClose(ctx context.Context, uri string) error {
	return c.Notify(ctx, "textDocument/didClose", DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
}

// RegisterNotification registers a handler for a notification
func (c *Client) RegisterNotification(method string, handler NotificationHandler) {
	c.muNotif.Lock()
	defer c.muNotif.Unlock()
	c.notifications[method] = handler
}

// Request sends a request and waits for response
func (c *Client) Request(ctx context.Context, method string, params interface{}, result interface{}) error {
	c.mu.Lock()
	id := c.nextID
	c.nextID++

	responseCh := make(chan *RPCMessage, 1)
	c.pending[id] = responseCh
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	// Build the request message
	msg := RPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
	}

	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
		msg.Params = paramsBytes
	}

	// Send the request
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Add content-length header for LSP
	content := string(msgBytes) + "\n"
	_, err = c.stdin.Write([]byte(content))
	if err != nil {
		return fmt.Errorf("failed to write request: %w", err)
	}

	// Wait for response
	select {
	case response := <-responseCh:
		if response.Error != nil {
			return response.Error
		}
		if result != nil && len(response.Result) > 0 {
			if err := json.Unmarshal(response.Result, result); err != nil {
				return fmt.Errorf("failed to unmarshal result: %w", err)
			}
		}
		return nil
	case <-c.readDone:
		return fmt.Errorf("connection closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Notify sends a notification (no response expected)
func (c *Client) Notify(ctx context.Context, method string, params interface{}) error {
	msg := RPCMessage{
		JSONRPC: "2.0",
		Method:  method,
	}

	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
		msg.Params = paramsBytes
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Add newline for LSP
	content := string(msgBytes) + "\n"
	_, err = c.stdin.Write([]byte(content))
	if err != nil {
		return fmt.Errorf("failed to write notification: %w", err)
	}

	return nil
}

func (c *Client) handleMessages() {
	decoder := json.NewDecoder(c.stdout)
	for {
		select {
		case <-c.readDone:
			return
		default:
			// No blocking - continue
		}

		var msg RPCMessage
		if err := decoder.Decode(&msg); err != nil {
			// Connection might be closed
			select {
			case <-c.readDone:
				return
			default:
				continue
			}
		}

		c.handleMessage(&msg)
	}
}

func (c *Client) handleMessage(msg *RPCMessage) {
	// Handle response (has ID)
	if msg.ID != nil {
		c.mu.RLock()
		id, ok := msg.ID.(float64)
		if ok {
			ch, exists := c.pending[int(id)]
			c.mu.RUnlock()
			if exists {
				select {
				case ch <- msg:
				default:
				}
			}
		} else {
			c.mu.RUnlock()
		}
		return
	}

	// Handle notification (no ID)
	c.muNotif.RLock()
	handler, exists := c.notifications[msg.Method]
	c.muNotif.RUnlock()

	if exists && handler != nil {
		handler(msg.Params)
	}
}

// Helper functions

func locationsFromResult(result interface{}) ([]Location, error) {
	if result == nil {
		return nil, nil
	}

	// Can be Location, []Location, or nil
	switch v := result.(type) {
	case []interface{}:
		locations := make([]Location, 0, len(v))
		for _, item := range v {
			if loc, ok := item.(map[string]interface{}); ok {
				locations = append(locations, locationFromMap(loc))
			}
		}
		return locations, nil
	case map[string]interface{}:
		return []Location{locationFromMap(v)}, nil
	}

	return nil, nil
}

func locationFromMap(m map[string]interface{}) Location {
	loc := Location{}
	if uri, ok := m["uri"].(string); ok {
		loc.URI = uri
	}
	if rng, ok := m["range"].(map[string]interface{}); ok {
		loc.Range = rangeFromMap(rng)
	}
	return loc
}

func rangeFromMap(m map[string]interface{}) Range {
	r := Range{}
	if start, ok := m["start"].(map[string]interface{}); ok {
		r.Start = positionFromMap(start)
	}
	if end, ok := m["end"].(map[string]interface{}); ok {
		r.End = positionFromMap(end)
	}
	return r
}

func positionFromMap(m map[string]interface{}) Position {
	p := Position{}
	if line, ok := m["line"].(float64); ok {
		p.Line = int(line)
	}
	if char, ok := m["character"].(float64); ok {
		p.Character = int(char)
	}
	return p
}

func documentSymbolFromMap(m map[string]interface{}) DocumentSymbol {
	sym := DocumentSymbol{}
	if name, ok := m["name"].(string); ok {
		sym.Name = name
	}
	if detail, ok := m["detail"].(string); ok {
		sym.Detail = detail
	}
	if kind, ok := m["kind"].(float64); ok {
		sym.Kind = int(kind)
	}
	if rng, ok := m["range"].(map[string]interface{}); ok {
		sym.Range = rangeFromMap(rng)
	}
	if selRng, ok := m["selectionRange"].(map[string]interface{}); ok {
		sym.SelectionRange = rangeFromMap(selRng)
	}
	if children, ok := m["children"].([]interface{}); ok {
		sym.Children = make([]DocumentSymbol, 0, len(children))
		for _, child := range children {
			if childMap, ok := child.(map[string]interface{}); ok {
				sym.Children = append(sym.Children, documentSymbolFromMap(childMap))
			}
		}
	}
	return sym
}
