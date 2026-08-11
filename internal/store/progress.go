package store

import "time"

// ApplyProgressUpdate merges an incoming progress write with the existing record.
// Rules mirror the previous Node implementation.
func ApplyProgressUpdate(
	existing map[string]interface{},
	tmdb interface{},
	timeVal, percentVal float64,
	fileID, deviceID interface{},
) map[string]interface{} {
	finalTime := timeVal
	finalPercent := percentVal

	if existing != nil {
		existDevice := StringifyID(existing["device_id"])
		reqDevice := StringifyID(deviceID)
		if reqDevice != "" && existDevice != "" && reqDevice == existDevice {
			finalTime = timeVal
			finalPercent = percentVal
		} else if reqDevice != "" && existDevice != "" && reqDevice != existDevice {
			finalTime = MaxFloat(NumOr(existing["time"], 0), timeVal)
			finalPercent = MaxFloat(NumOr(existing["percent"], 0), percentVal)
		} else if upd, ok := existing["updated"].(string); ok && upd != "" {
			if t, err := time.Parse(time.RFC3339Nano, upd); err == nil {
				if time.Since(t) < 5*time.Second {
					finalTime = MaxFloat(NumOr(existing["time"], 0), timeVal)
					finalPercent = MaxFloat(NumOr(existing["percent"], 0), percentVal)
				}
			} else if t, err := time.Parse(time.RFC3339, upd); err == nil {
				if time.Since(t) < 5*time.Second {
					finalTime = MaxFloat(NumOr(existing["time"], 0), timeVal)
					finalPercent = MaxFloat(NumOr(existing["percent"], 0), percentVal)
				}
			}
		}
	}

	fileMapping := map[string]interface{}{}
	if existing != nil {
		for k, v := range MapOrEmpty(existing["file_mapping"]) {
			fileMapping[k] = v
		}
	}
	if fileID != nil && StringifyID(fileID) != "" {
		fileMapping[StringifyID(fileID)] = tmdb
	}

	outDevice := deviceID
	if StringifyID(outDevice) == "" {
		if existing != nil {
			outDevice = existing["device_id"]
		} else {
			outDevice = nil
		}
	}

	return map[string]interface{}{
		"time":         finalTime,
		"percent":      finalPercent,
		"file_mapping": fileMapping,
		"device_id":    outDevice,
		"updated":      time.Now().UTC().Format(time.RFC3339Nano),
	}
}

// FindProgressByFileID looks up a progress record by file_mapping key.
func FindProgressByFileID(progress map[string]interface{}, fileID string) (tmdb string, record map[string]interface{}, ok bool) {
	for key, raw := range progress {
		rec, isMap := raw.(map[string]interface{})
		if !isMap {
			continue
		}
		mapping := MapOrEmpty(rec["file_mapping"])
		if _, found := mapping[fileID]; found {
			return key, rec, true
		}
	}
	return "", nil, false
}
