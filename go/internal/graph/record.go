// Package graph provides database abstraction for graph operations.
// record.go consolidates 36 duplicate type-extraction helpers.
package graph

// GetString extracts a string value from a Record.
// Eliminates 10 duplicate getString functions across packages.
func GetString(r Record, key string) string {
	if v, ok := r[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt extracts an int value from a Record.
// Handles int, int64, and float64 (truncated).
// Eliminates 8 duplicate getInt functions across packages.
func GetInt(r Record, key string) int {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case int64:
			return int(n)
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return 0
}

// GetInt64 extracts an int64 value from a Record.
// Eliminates 5 duplicate getInt64 functions across packages.
func GetInt64(r Record, key string) int64 {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return 0
}

// GetFloat extracts a float64 value from a Record.
// Eliminates 4 duplicate getFloat functions across packages.
func GetFloat(r Record, key string) float64 {
	if v, ok := r[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int64:
			return float64(n)
		case int:
			return float64(n)
		}
	}
	return 0.0
}

// GetBool extracts a bool value from a Record.
// Eliminates 2 duplicate getBool functions across packages.
func GetBool(r Record, key string) bool {
	if v, ok := r[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// GetStringSlice extracts a []string value from a Record.
// Handles both []string and []any containing strings.
func GetStringSlice(r Record, key string) []string {
	if v, ok := r[key]; ok {
		switch s := v.(type) {
		case []string:
			return s
		case []any:
			result := make([]string, 0, len(s))
			for _, item := range s {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}
