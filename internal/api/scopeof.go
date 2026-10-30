package api

import (
	"encoding/json"

	"printmarshal/internal/model"
)

// deviceIDOf extracts the device a record or event payload is about, or ""
// when it is not about one device. Scoped readers (the sync API, the live
// stream) use it to drop rows outside the principal's groups.
func deviceIDOf(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case *model.Device:
		return x.ID
	case model.Device:
		return x.ID
	case *model.Alert:
		return x.DeviceID
	case model.Alert:
		return x.DeviceID
	case map[string]any:
		if id, _ := x["deviceId"].(string); id != "" {
			return id
		}
		switch d := x["device"].(type) {
		case string:
			return d
		case map[string]any:
			id, _ := d["id"].(string)
			return id
		}
		return ""
	}
	// Other records: look for the conventional fields after a JSON round trip.
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	var probe struct {
		DeviceID string `json:"deviceId"`
		Device   struct {
			ID string `json:"id"`
		} `json:"device"`
		Result struct {
			DeviceID string `json:"deviceId"`
		} `json:"result"`
	}
	if json.Unmarshal(b, &probe) != nil {
		return ""
	}
	switch {
	case probe.DeviceID != "":
		return probe.DeviceID
	case probe.Device.ID != "":
		return probe.Device.ID
	case probe.Result.DeviceID != "":
		return probe.Result.DeviceID
	}
	return ""
}
