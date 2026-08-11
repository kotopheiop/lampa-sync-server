package store

import (
	"fmt"
	"strconv"
)

// FavoriteKeys is the ordered list of Lampa favorite arrays.
var FavoriteKeys = []string{
	"card", "like", "watch", "wath", "book", "history",
	"look", "viewed", "scheduled", "continued", "thrown",
}

// EmptyFavorite returns a blank favorite object.
func EmptyFavorite() map[string]interface{} {
	out := map[string]interface{}{"updated": nil}
	for _, k := range FavoriteKeys {
		out[k] = []interface{}{}
	}
	return out
}

// NormalizeArray normalizes favorite list items and de-duplicates.
func NormalizeArray(arr interface{}) []interface{} {
	slice, ok := arr.([]interface{})
	if !ok || slice == nil {
		return []interface{}{}
	}
	out := make([]interface{}, 0, len(slice))
	seen := map[string]struct{}{}
	for _, item := range slice {
		if item == nil {
			continue
		}
		var norm interface{}
		switch v := item.(type) {
		case float64, bool:
			norm = v
		case string:
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				norm = float64(n)
			} else {
				norm = v
			}
		case map[string]interface{}:
			if id, ok := v["id"]; ok {
				norm = id
			} else if id, ok := v["tmdb_id"]; ok {
				norm = id
			} else {
				norm = v
			}
		default:
			norm = v
		}
		key := fmt.Sprintf("%T:%v", norm, norm)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, norm)
	}
	return out
}

// NormalizeFavoriteObject ensures all favorite keys exist and are normalized.
func NormalizeFavoriteObject(favorite map[string]interface{}) map[string]interface{} {
	out := EmptyFavorite()
	if favorite == nil {
		return out
	}
	for _, key := range FavoriteKeys {
		out[key] = NormalizeArray(favorite[key])
	}
	// Lampa uses "wath"; keep watch↔wath mirrored
	mirrored := NormalizeArray(append(AsSlice(favorite["wath"]), AsSlice(favorite["watch"])...))
	out["wath"] = mirrored
	out["watch"] = mirrored
	if u, ok := favorite["updated"]; ok {
		out["updated"] = u
	}
	return out
}

// MergeFavorite unions favorite lists from a and b.
func MergeFavorite(a, b map[string]interface{}) map[string]interface{} {
	if a == nil {
		return NormalizeFavoriteObject(b)
	}
	if b == nil {
		return NormalizeFavoriteObject(a)
	}
	merged := EmptyFavorite()
	for _, key := range FavoriteKeys {
		merged[key] = NormalizeArray(append(AsSlice(a[key]), AsSlice(b[key])...))
	}
	return merged
}

// BuildProgressSummary maps progress records to the /sync payload shape.
func BuildProgressSummary(progress map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for tmdb, raw := range progress {
		rec, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		out[tmdb] = map[string]interface{}{
			"time":         NumOr(rec["time"], 0),
			"percent":      NumOr(rec["percent"], 0),
			"file_mapping": MapOrEmpty(rec["file_mapping"]),
			"updated":      rec["updated"],
			"device_id":    rec["device_id"],
		}
	}
	return out
}

// AsSlice coerces v to []interface{}.
func AsSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok && s != nil {
		return s
	}
	return []interface{}{}
}

// MapOrEmpty coerces v to map[string]interface{}.
func MapOrEmpty(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok && m != nil {
		return m
	}
	return map[string]interface{}{}
}

// NumOr coerces v to float64 or returns def.
func NumOr(v interface{}, def float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return f
		}
	}
	return def
}

// StringifyID converts ids used in JSON payloads to a stable string key.
func StringifyID(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		s := fmt.Sprint(v)
		if s == "<nil>" {
			return ""
		}
		return s
	}
}

// MaxFloat returns the larger of a and b.
func MaxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
