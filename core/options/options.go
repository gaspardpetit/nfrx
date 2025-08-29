package options

import "strconv"

// String returns a plugin option value or the provided default when absent.
func String(pluginOptions map[string]map[string]string, pluginID, key, def string) string {
	if pluginOptions == nil {
		return def
	}
	if po, ok := pluginOptions[pluginID]; ok {
		if v, ok := po[key]; ok && v != "" {
			return v
		}
	}
	return def
}

// Int parses and returns a plugin option as int, falling back to def on error/absence.
func Int(pluginOptions map[string]map[string]string, pluginID, key string, def int) int {
	v := String(pluginOptions, pluginID, key, "")
	if v == "" {
		return def
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return def
}

// Int64 parses and returns a plugin option as int64, falling back to def on error/absence.
func Int64(pluginOptions map[string]map[string]string, pluginID, key string, def int64) int64 {
	v := String(pluginOptions, pluginID, key, "")
	if v == "" {
		return def
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	return def
}

// Bool parses and returns a plugin option as bool, falling back to def on error/absence.
func Bool(pluginOptions map[string]map[string]string, pluginID, key string, def bool) bool {
	v := String(pluginOptions, pluginID, key, "")
	if v == "" {
		return def
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return def
}

// Float parses and returns a plugin option as float64, falling back to def on error/absence.
func Float(pluginOptions map[string]map[string]string, pluginID, key string, def float64) float64 {
	v := String(pluginOptions, pluginID, key, "")
	if v == "" {
		return def
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		return n
	}
	return def
}
