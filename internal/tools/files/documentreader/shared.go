// Package documentreader groups the explicit, agent-invocable tools built on a
// document-conversion backend: read_document_url (fetch+convert a remote
// document) and convert_document (convert a local file that native
// extraction can't handle well - scanned PDFs, complex slide decks, audio).
// Both are opt-in: the agent reaches for them deliberately, as opposed to
// internal/tools/files/read's own conversion fallback, which stays in place
// so a scanned PDF or audio file still "just works" through plain FileRead
// without the agent needing to know this package exists.
package documentreader

import internalreader "github.com/KPO-Tech/seshat/internal/documentreader"

// Config holds the shared configuration for every tool in this package.
type Config struct {
	// DocumentReaderURL is the base URL of a document-reader service. Used
	// to build the default DocumentConverter when that field is left nil.
	// When both are empty/nil, tools register but always report "not
	// configured".
	DocumentReaderURL string

	// DocumentConverter, when set, is used instead of building a
	// service client from DocumentReaderURL. Takes precedence over
	// DocumentReaderURL.
	DocumentConverter internalreader.Converter
}

// converter resolves the backend to use: the explicit override if set,
// otherwise a service client built from DocumentReaderURL.
func (c Config) converter() internalreader.Converter {
	if c.DocumentConverter != nil {
		return c.DocumentConverter
	}
	if c.DocumentReaderURL != "" {
		return internalreader.NewDoclingClient(c.DocumentReaderURL)
	}
	return nil
}
