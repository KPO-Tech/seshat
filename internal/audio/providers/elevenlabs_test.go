package audioproviders

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestElevenLabsTTSUsesConfiguredVoiceAndFormat(t *testing.T) {
	var path, query, key, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query, key = r.URL.Path, r.URL.RawQuery, r.Header.Get("xi-api-key")
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		_, _ = w.Write([]byte("AUDIO"))
	}))
	defer srv.Close()

	c := NewElevenLabsTTS("secret", WithElevenLabsTTSBaseURL(srv.URL+"/v1/"), WithElevenLabsTTSVoice("voice1"), WithElevenLabsTTSFormat("wav"))
	got, err := c.GenerateAudio(context.Background(), "bonjour")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/text-to-speech/voice1" || query != "output_format=wav_44100" || key != "secret" {
		t.Errorf("request = %s ?%s key=%q", path, query, key)
	}
	if !strings.Contains(body, `"model_id":"eleven_multilingual_v2"`) || !strings.Contains(body, `"text":"bonjour"`) {
		t.Errorf("body = %s", body)
	}
	if string(got.AudioData) != "AUDIO" || got.ContentType != "audio/wav" || got.CharactersUsed != 7 {
		t.Errorf("response = %+v", got)
	}
	if c.Provider() != "elevenlabs" {
		t.Errorf("Provider() = %q", c.Provider())
	}
}

func TestElevenLabsTTSWithoutVoiceTakesTheFirstOfTheAccount(t *testing.T) {
	var voiceCalls int
	var spoken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/voices" {
			voiceCalls++
			_, _ = w.Write([]byte(`{"voices":[{"voice_id":"first","name":"Ada","labels":{"language":"fr"}},{"voice_id":"second","name":"Bob"}]}`))
			return
		}
		spoken = r.URL.Path
		_, _ = w.Write([]byte("A"))
	}))
	defer srv.Close()
	c := NewElevenLabsTTS("k", WithElevenLabsTTSBaseURL(srv.URL))
	for i := 0; i < 2; i++ {
		if _, err := c.GenerateAudio(context.Background(), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if spoken != "/v1/text-to-speech/first" || voiceCalls != 1 {
		t.Errorf("spoken=%q voiceCalls=%d (the voice must be looked up once)", spoken, voiceCalls)
	}
	voices, err := c.ListVoices(context.Background())
	if err != nil || len(voices) != 2 || voices[0].Name != "Ada" || voices[0].Language != "fr" {
		t.Errorf("voices = %+v err=%v", voices, err)
	}
}

func TestElevenLabsTTSReportsAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"bad key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewElevenLabsTTS("k", WithElevenLabsTTSBaseURL(srv.URL), WithElevenLabsTTSVoice("v"))
	if _, err := c.GenerateAudio(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected a 401 error, got %v", err)
	}
}

func TestElevenLabsSTTReadsWordsAndSkipsSpacing(t *testing.T) {
	fields := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/speech-to-text" || r.Header.Get("xi-api-key") != "k" {
			t.Errorf("request = %s key=%q", r.URL.Path, r.Header.Get("xi-api-key"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
			return
		}
		for name, v := range r.MultipartForm.Value {
			fields[name] = v[0]
		}
		_, _ = w.Write([]byte(`{"language_code":"fr","text":"bonjour toi","words":[
			{"text":"bonjour","type":"word","start":0.1,"end":0.6},
			{"text":" ","type":"spacing","start":0.6,"end":0.7},
			{"text":"toi","type":"word","start":0.7,"end":1.2}]}`))
	}))
	defer srv.Close()
	c := NewElevenLabsSTT("k", WithElevenLabsSTTBaseURL(srv.URL), WithElevenLabsSTTLanguage("fr"))
	got, err := c.Transcribe(context.Background(), []byte("audio"))
	if err != nil {
		t.Fatal(err)
	}
	if fields["model_id"] != "scribe_v2" || fields["language_code"] != "fr" {
		t.Errorf("form = %v", fields)
	}
	if got.Text != "bonjour toi" || got.Language != "fr" || len(got.Words) != 2 || got.Duration != 1.2 {
		t.Errorf("response = %+v", got)
	}
}

func TestElevenLabsOutputFormats(t *testing.T) {
	cases := map[string][2]string{
		"":              {"mp3_44100_128", "audio/mpeg"},
		"MP3":           {"mp3_44100_128", "audio/mpeg"},
		"opus":          {"opus_48000_128", "audio/ogg"},
		"pcm":           {"pcm_24000", "audio/pcm"},
		"mp3_44100_192": {"mp3_44100_192", "audio/mpeg"},
		"ulaw_8000":     {"ulaw_8000", "audio/basic"},
	}
	for in, want := range cases {
		if f, ct := elevenLabsOutput(in); f != want[0] || ct != want[1] {
			t.Errorf("%q -> %q %q, want %v", in, f, ct, want)
		}
	}
}
