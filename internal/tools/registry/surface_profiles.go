package registry

const (
	ToolSurfaceProfileMonoRun    = "mono_run"
	ToolSurfaceProfileSubagent   = "subagent"
	ToolSurfaceProfileSkillAgent = "skill_agent"
)

const toolSurfaceProfilesMetadataKey = "surface_profiles"

func visibleInSurfaceProfile(def ToolDefinition, profile string) bool {
	if profile == "" {
		return true
	}
	rawProfiles, ok := def.Metadata[toolSurfaceProfilesMetadataKey]
	if !ok || rawProfiles == nil {
		return true
	}
	for _, candidate := range normalizeSurfaceProfiles(rawProfiles) {
		if candidate == profile {
			return true
		}
	}
	return false
}

func VisibleInSurfaceProfile(def ToolDefinition, profile string) bool {
	return visibleInSurfaceProfile(def, profile)
}

func normalizeSurfaceProfiles(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		profiles := make([]string, 0, len(values))
		for _, value := range values {
			s, ok := value.(string)
			if ok && s != "" {
				profiles = append(profiles, s)
			}
		}
		return profiles
	default:
		return nil
	}
}
