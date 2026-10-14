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
	t.Setenv("OPENAI_API_KEY", "")
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
	t.Setenv("OPENAI_API_KEY", "")
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

func TestAudioEndpoint_CatalogProviderUsesItsOwnDefaults(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk-test")
	cfg := &ClientConfig{SpeechToText: &SpeechToTextConfig{Provider: "groq"}}
	var out string
	captureLogInto(&out, func() {
		gen := initSpeechToTextTranscriber(cfg)
		if gen == nil {
			t.Fatal("expected a transcriber for groq with its key in the environment")
		}
		if gen.Provider() != "groq" {
			t.Fatalf("Provider() = %q, want groq", gen.Provider())
		}
	})
	if out != "" {
		t.Fatalf("expected no warning, got %q", out)
	}
}

func TestAudioEndpoint_ProviderWithoutThatDirectionStaysDisabled(t *testing.T) {
	cfg := &ClientConfig{TextToSpeech: &TextToSpeechConfig{Provider: "groq", APIKey: "k"}}
	var out string
	captureLogInto(&out, func() {
		if gen := initTextToSpeechGenerator(cfg); gen != nil {
			t.Fatalf("groq has no text-to-speech in the catalog, got %v", gen)
		}
	})
	if !strings.Contains(out, "does not offer it") {
		t.Fatalf("expected a warning, got %q", out)
	}
}

func TestAudioEndpoint_UnknownProviderNeedsABaseURL(t *testing.T) {
	var out string
	captureLogInto(&out, func() {
		cfg := &ClientConfig{SpeechToText: &SpeechToTextConfig{Provider: "acme", APIKey: "k"}}
		if gen := initSpeechToTextTranscriber(cfg); gen != nil {
			t.Fatalf("unknown provider without a base URL must stay disabled, got %v", gen)
		}
	})
	if !strings.Contains(out, "unrecognized provider") {
		t.Fatalf("expected an unrecognized-provider warning, got %q", out)
	}
	cfg := &ClientConfig{SpeechToText: &SpeechToTextConfig{Provider: "acme", BaseURL: "http://localhost:9000"}}
	gen := initSpeechToTextTranscriber(cfg)
	if gen == nil || gen.Provider() != "acme" {
		t.Fatalf("a custom endpoint should work without a key, got %v", gen)
	}
}

func TestAudioEndpoint_ElevenLabsFromCatalog(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "xi-test")
	speak := initTextToSpeechGenerator(&ClientConfig{TextToSpeech: &TextToSpeechConfig{Provider: "elevenlabs", Voice: "v1"}})
	listen := initSpeechToTextTranscriber(&ClientConfig{SpeechToText: &SpeechToTextConfig{Provider: "ElevenLabs"}})
	if speak == nil || speak.Provider() != "elevenlabs" || listen == nil || listen.Provider() != "elevenlabs" {
		t.Fatalf("expected ElevenLabs clients from the catalog, got %v / %v", speak, listen)
	}
}
