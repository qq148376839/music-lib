package utils

import (
	"fmt"
	"strconv"
)

// ParseAnyInt converts an any value (typically from JSON unmarshaling into interface{})
// to int. Handles float64, int, and string types.
func ParseAnyInt(val any) int {
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		if v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
	}
	return 0
}

// ParseAnyInt64 converts an any value to int64.
// Handles float64, int, int64, and string types.
func ParseAnyInt64(val any) int64 {
	switch v := val.(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case string:
		if v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n
			}
		}
	}
	return 0
}

// ParseAnyString converts an any value to string.
// Handles string, float64, and int types.
func ParseAnyString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		if val != nil {
			return fmt.Sprintf("%v", val)
		}
	}
	return ""
}
