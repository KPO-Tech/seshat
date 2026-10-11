package skills

import (
	"strings"
	"testing"
)

func TestFormatSkillsList_IncludesWhenToUse(t *testing.T) {
	skills := []Skill{
		{Name: "greet", Description: "says hello", WhenToUse: "when the user wants a greeting"},
	}
	got := FormatSkillsList(skills)
	if !strings.Contains(got, "when the user wants a greeting") {
		t.Fatalf("expected when_to_use to appear in the formatted list, got: %q", got)
	}
}

func TestFormatSkillsList_OmitsWhenToUseIfEmpty(t *testing.T) {
	skills := []Skill{{Name: "greet", Description: "says hello"}}
	got := FormatSkillsList(skills)
	if !strings.Contains(got, "greet: says hello") {
		t.Fatalf("expected the basic name/description line, got: %q", got)
	}
}

func TestParseArgumentNames_StructuredForm(t *testing.T) {
	value := []interface{}{
		map[string]interface{}{"name": "file", "description": "path to process"},
		map[string]interface{}{"name": "format", "description": "output format", "default": "text"},
	}
	got := ParseArgumentNames(value)
	want := []string{"file", "format"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestParseArgumentNames_StructuredFormSkipsEntryWithoutName(t *testing.T) {
	value := []interface{}{
		map[string]interface{}{"description": "no name field"},
		map[string]interface{}{"name": "format"},
	}
	got := ParseArgumentNames(value)
	if len(got) != 1 || got[0] != "format" {
		t.Fatalf("expected only 'format', got %v", got)
	}
}

func TestParseArgumentNames_MixedStringAndStructuredForm(t *testing.T) {
	value := []interface{}{
		"file",
		map[string]interface{}{"name": "format"},
	}
	got := ParseArgumentNames(value)
	want := []string{"file", "format"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestSubstituteArguments_SingleNameGetsWholeString(t *testing.T) {
	got := SubstituteArguments("Process ${argument:file} now", "my file.txt", []string{"file"})
	if got != "Process my file.txt now" {
		t.Fatalf("expected the whole args string in the single placeholder, got: %q", got)
	}
}

func TestSubstituteArguments_MultipleNamesSplitPositionally(t *testing.T) {
	got := SubstituteArguments(
		"file=${argument:file} format=${argument:format}",
		"notes.txt json",
		[]string{"file", "format"},
	)
	if got != "file=notes.txt format=json" {
		t.Fatalf("expected positional split across both placeholders, got: %q", got)
	}
}

func TestSubstituteArguments_LastNameAbsorbsRemainder(t *testing.T) {
	got := SubstituteArguments(
		"scope=${argument:scope} message=${argument:message}",
		"api fix the thing that was broken",
		[]string{"scope", "message"},
	)
	if got != "scope=api message=fix the thing that was broken" {
		t.Fatalf("expected the last name to absorb the remaining words, got: %q", got)
	}
}

func TestSubstituteArguments_MissingTrailingValueBecomesEmpty(t *testing.T) {
	got := SubstituteArguments(
		"file=${argument:file} format=[${argument:format}]",
		"notes.txt",
		[]string{"file", "format"},
	)
	if got != "file=notes.txt format=[]" {
		t.Fatalf("expected the unfilled trailing name to become empty, got: %q", got)
	}
}

func TestSubstituteArguments_BareNamePlaceholderAlsoSubstituted(t *testing.T) {
	got := SubstituteArguments("Hello ${name}", "World", []string{"name"})
	if got != "Hello World" {
		t.Fatalf("expected the bare ${name} form to be substituted too, got: %q", got)
	}
}

func TestSubstituteArguments_NoArgsReturnsContentUnchanged(t *testing.T) {
	got := SubstituteArguments("Hello ${argument:name}", "", []string{"name"})
	if got != "Hello ${argument:name}" {
		t.Fatalf("expected content unchanged when args is empty, got: %q", got)
	}
}
