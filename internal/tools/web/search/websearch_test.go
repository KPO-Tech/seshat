package websearch

import "testing"

func TestDefinitionExposesSnakeCaseAlias(t *testing.T) {
	def := NewTool().Definition()
	found := false
	for _, alias := range def.Aliases {
		if alias == "web_search" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected web_search alias, got %v", def.Aliases)
	}
}
