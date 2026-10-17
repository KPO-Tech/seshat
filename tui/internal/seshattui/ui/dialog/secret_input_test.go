package dialog

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/ui/common"
	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/ui/styles"
)

// The API key dialogs hand a user's key to a secret input, which must render
// bullets instead of the key.
func TestSecretInputHidesValue(t *testing.T) {
	in := newSecretInput()
	in.SetWidth(60)
	in.SetValue("sk-live-supersecret-1234")

	if in.EchoMode != textinput.EchoPassword {
		t.Fatalf("EchoMode = %v, want EchoPassword", in.EchoMode)
	}
	if in.EchoCharacter != SecretMask {
		t.Fatalf("EchoCharacter = %q, want %q", in.EchoCharacter, SecretMask)
	}

	view := in.View()
	if strings.Contains(view, "supersecret") || strings.Contains(view, "sk-live") {
		t.Fatalf("secret input renders its value: %q", view)
	}
	if !strings.Contains(view, string(SecretMask)) {
		t.Fatalf("secret input does not render %q: %q", SecretMask, view)
	}
}

// The value stays hidden when the input is not focused, which is the state the
// dialog shows once the configuration has been validated.
func TestSecretInputHidesValueWhenBlurred(t *testing.T) {
	in := newSecretInput()
	in.SetWidth(60)
	in.SetValue("another-secret-value")
	in.Blur()

	if view := in.View(); strings.Contains(view, "another-secret-value") {
		t.Fatalf("blurred secret input renders its value: %q", view)
	}
}

// Once a provider configuration is validated the key is masked again, even if
// the user had revealed it while typing.
func TestAPIKeyInputMasksKeyOnceValidated(t *testing.T) {
	m := &APIKeyInput{
		com:       &common.Common{Styles: &styles.Styles{}},
		hasAPIKey: true,
	}
	m.apiKeyInput = newSecretInput()
	m.apiKeyInput.SetWidth(60)
	m.apiKeyInput.SetValue("sk-live-abcdef123456")

	m.setAPIKeyVisible(true)
	if !m.apiKeyVisible() {
		t.Fatal("API key is hidden right after being revealed")
	}

	m.HandleMsg(ActionChangeAPIKeyState{State: APIKeyInputStateVerifying})
	m.HandleMsg(ActionChangeAPIKeyState{State: APIKeyInputStateVerified})

	if m.apiKeyVisible() {
		t.Fatal("API key is still visible after validation")
	}
	if view := m.apiKeyInput.View(); strings.Contains(view, "abcdef") {
		t.Fatalf("validated API key is rendered: %q", view)
	}
}
