package vector

// matchesFilter returns true when r.Metadata satisfies every predicate in filter.
// Supported predicates:
//
//	{"key": "value"}              — simple equality
//	{"key": {"$in": ["a","b"]}}  — membership test
func matchesFilter(r Record, filter map[string]any) bool {
	for k, v := range filter {
		switch t := v.(type) {
		case string:
			if r.Metadata[k] != t {
				return false
			}
		case map[string]any:
			if ins, ok := t["$in"]; ok {
				if !inList(r.Metadata[k], ins) {
					return false
				}
			}
		}
	}
	return true
}

func inList(val string, list any) bool {
	switch s := list.(type) {
	case []string:
		for _, v := range s {
			if v == val {
				return true
			}
		}
	case []any:
		for _, v := range s {
			if str, ok := v.(string); ok && str == val {
				return true
			}
		}
	}
	return false
}
