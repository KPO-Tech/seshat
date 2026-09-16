package msgraph

import (
	"net/http/httptest"
	"testing"
)

// swapGraphBaseURLForTest points graphBaseURL at server for the duration of
// the test, restoring the real value on cleanup - lets tests exercise the
// real ListMessagesDelta/ListChatMessages/etc. bootstrap paths (which build
// their first request off graphBaseURL) against a fake Graph server, not
// just the "resume from an already-absolute cursor URL" path. Thin wrapper
// over the exported SetBaseURLForTesting so this package's own tests and
// external consumers' tests share one mechanism.
func swapGraphBaseURLForTest(t *testing.T, server *httptest.Server) {
	t.Helper()
	restore := SetBaseURLForTesting(server.URL)
	t.Cleanup(restore)
}
