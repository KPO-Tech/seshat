package workspace

import (
	"errors"
	"strings"
	"testing"
)

func TestFinishErrorTextCarriesTheRealError(t *testing.T) {
	title, details := finishErrorText(errors.New("provider returned 429\nretry after 30s"))
	if title != "provider returned 429" {
		t.Errorf("the title is the first line of the error, got %q", title)
	}
	if !strings.Contains(details, "retry after 30s") {
		t.Errorf("the details keep the whole error, got %q", details)
	}
}

func TestFinishErrorTextNeverReturnsAnEmptyTitle(t *testing.T) {
	for _, err := range []error{nil, errors.New(""), errors.New("  \n")} {
		if title, _ := finishErrorText(err); strings.TrimSpace(title) == "" {
			t.Errorf("an empty error must still give a readable title (err=%v)", err)
		}
	}
}
