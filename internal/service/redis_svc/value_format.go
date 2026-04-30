package redis_svc

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// FormatDisplayValue formats a raw Redis string for display. It never changes
// the value that will be saved back unless the caller explicitly asks
// EncodeValueForStorage to decode an encoded edit mode.
func FormatDisplayValue(raw string, format string) RedisFormattedValue {
	switch format {
	case RedisValueFormatJSON:
		var data any
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return RedisFormattedValue{Format: format, Value: raw, Valid: false, Error: err.Error()}
		}
		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return RedisFormattedValue{Format: format, Value: raw, Valid: false, Error: err.Error()}
		}
		return RedisFormattedValue{Format: format, Value: string(out), Valid: true}
	case RedisValueFormatHex:
		return RedisFormattedValue{Format: format, Value: hex.EncodeToString([]byte(raw)), Valid: true}
	case RedisValueFormatBase64:
		return RedisFormattedValue{Format: format, Value: base64.StdEncoding.EncodeToString([]byte(raw)), Valid: true}
	case RedisValueFormatRaw:
		return RedisFormattedValue{Format: format, Value: raw, Valid: true}
	default:
		return RedisFormattedValue{Format: RedisValueFormatRaw, Value: raw, Valid: true}
	}
}

// EncodeValueForStorage converts explicit encoded edit modes back to the raw
// Redis string. JSON mode intentionally stores the exact editor text.
func EncodeValueForStorage(value string, format string) (string, error) {
	switch format {
	case RedisValueFormatHex:
		data, err := hex.DecodeString(value)
		if err != nil {
			return "", fmt.Errorf("invalid hex value: %w", err)
		}
		return string(data), nil
	case RedisValueFormatBase64:
		data, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return "", fmt.Errorf("invalid base64 value: %w", err)
		}
		return string(data), nil
	case RedisValueFormatRaw, RedisValueFormatJSON, "":
		return value, nil
	default:
		return value, nil
	}
}
