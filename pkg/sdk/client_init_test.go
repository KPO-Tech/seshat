package sdk

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// captureLogInto redirects the standard logger for the duration of fn and
// writes everything logged during fn into *out.
func captureLogInto(out *string, fn func()) {
	var buf bytes.Buffer
	origOutput := log.Writer()
	origFlags := log.Flags()
	origPrefix := log.Prefix()
	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(origOutput)
		log.SetFlags(origFlags)
		log.SetPrefix(origPrefix)
	}()
	fn()
	*out = buf.String()
}

func TestInitImageGenerator_UnconfiguredStaysSilent(t *testing.T) {
	cfg := &ClientConfig{}
	var out string
	captureLogInto(&out, func() {
		if gen := initImageGenerator(cfg); gen != nil {
			t.Fatalf("expected nil generator when ImageGeneration is unset, got %v", gen)
		}
	})
	if out != "" {
		t.Fatalf("expected no log output for unconfigured capability, got: %q", out)
	}
}

func TestInitImageGenerator_ConfiguredWithoutCredentialsWarns(t *testing.T) {
	cfg := &ClientConfig{
		ImageGeneration: &ImageGenerationConfig{Provider: "openai"},
	}
	var out string
	captureLogInto(&out, func() {
		if gen := initImageGenerator(cfg); gen != nil {
			t.Fatalf("expected nil generator when no credentials resolve, got %v", gen)
		}
	})
	if !strings.Contains(out, "generate_image") || !strings.Contains(out, "openai") {
		t.Fatalf("expected a warning naming the tool and provider, got: %q", out)
	}
}

func TestInitImageGenerator_UnknownProviderWarns(t *testing.T) {
	cfg := &ClientConfig{
		ImageGeneration: &ImageGenerationConfig{Provider: "not-a-real-provider"},
	}
	var out string
	captureLogInto(&out, func() {
		if gen := initImageGenerator(cfg); gen != nil {
			t.Fatalf("expected nil generator for an unknown provider, got %v", gen)
		}
	})
	if !strings.Contains(out, "unrecognized provider") {
		t.Fatalf("expected an unrecognized-provider warning, got: %q", out)
	}
}

func TestInitImageGenerator_ConfiguredWithKeyStaysSilent(t *testing.T) {
	cfg := &ClientConfig{
		ImageGeneration: &ImageGenerationConfig{Provider: "openai", APIKey: "sk-test"},
	}
	var out string
	captureLogInto(&out, func() {
		if gen := initImageGenerator(cfg); gen == nil {
			t.Fatalf("expected a generator once an API key is resolved")
		}
	})
	if out != "" {
		t.Fatalf("expected no warning once credentials resolve, got: %q", out)
	}
}

func TestInitTextToSpeechGenerator_ConfiguredWithoutCredentialsWarns(t *testing.T) {
	cfg := &ClientConfig{
		TextToSpeech: &TextToSpeechConfig{Provider: "openai"},
	}
	var out string
	captureLogInto(&out, func() {
		if gen := initTextToSpeechGenerator(cfg); gen != nil {
			t.Fatalf("expected nil generator when no credentials resolve, got %v", gen)
		}
	})
	if !strings.Contains(out, "text_to_speech") || !strings.Contains(out, "openai") {
		t.Fatalf("expected a warning naming the tool and provider, got: %q", out)
	}
}

func TestInitSpeechToTextTranscriber_ConfiguredWithoutCredentialsWarns(t *testing.T) {
	cfg := &ClientConfig{
		SpeechToText: &SpeechToTextConfig{Provider: "openai"},
	}
	var out string
	captureLogInto(&out, func() {
		if gen := initSpeechToTextTranscriber(cfg); gen != nil {
			t.Fatalf("expected nil transcriber when no credentials resolve, got %v", gen)
		}
	})
	if !strings.Contains(out, "speech_to_text") || !strings.Contains(out, "openai") {
		t.Fatalf("expected a warning naming the tool and provider, got: %q", out)
	}
}

func TestInitSpeechToTextTranscriber_UnconfiguredStaysSilent(t *testing.T) {
	cfg := &ClientConfig{}
	var out string
	captureLogInto(&out, func() {
		if gen := initSpeechToTextTranscriber(cfg); gen != nil {
			t.Fatalf("expected nil transcriber when SpeechToText is unset, got %v", gen)
		}
	})
	if out != "" {
		t.Fatalf("expected no log output for unconfigured capability, got: %q", out)
	}
}
