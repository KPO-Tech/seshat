// Package docling groups the explicit, agent-invocable tools built on a
// document-conversion backend: read_document_url (fetch+convert a remote
// document) and docling_convert (convert a local file that native
// extraction can't handle well - scanned PDFs, complex slide decks, audio).
// Both are opt-in: the agent reaches for them deliberately, as opposed to
// internal/tools/files/read's own conversion fallback, which stays in place
// so a scanned PDF or audio file still "just works" through plain FileRead
// without the agent needing to know this package exists.
package docling

import internaldocling "github.com/KPO-Tech/seshat/internal/docling"

// Config holds the shared configuration for every tool in this package.
type Config struct {
	// DoclingURL is the base URL of a running docling-serve instance. Used
	// to build the default DocumentConverter when that field is left nil.
	// When both are empty/nil, tools register but always report "not
	// configured".
	DoclingURL string
	// DocumentConverter, when set, is used instead of building a
	// docling-serve client from DoclingURL - inject a custom
	// internaldocling.DocumentConverterBackend implementation to back these
	// tools with something other than docling-serve. Takes precedence over
	// DoclingURL.
	DocumentConverter internaldocling.DocumentConverterBackend
}

// converter resolves the backend to use: the explicit override if set,
// otherwise a docling-serve client built from DoclingURL, otherwise nil.
func (c Config) converter() internaldocling.DocumentConverterBackend {
	if c.DocumentConverter != nil {
		return c.DocumentConverter
	}
	if c.DoclingURL != "" {
		return internaldocling.NewClient(c.DoclingURL)
	}
	return nil
}
