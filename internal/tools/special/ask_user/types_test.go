package askuser

import "testing"

func TestInputValidate_AllowsUpToTenOptions(t *testing.T) {
	input := &Input{
		Questions: []Question{
			{
				Question: "Quels sont vos centres d'intérêt ?",
				Header:   "Interests",
				Options: []QuestionOption{
					{Label: "IA"},
					{Label: "Web"},
					{Label: "Mobile"},
					{Label: "Jeux"},
					{Label: "Data"},
					{Label: "Sécurité"},
					{Label: "IoT"},
				},
			},
		},
	}

	if err := input.Validate(); err != nil {
		t.Fatalf("expected 7 options to be accepted, got error: %v", err)
	}
}
