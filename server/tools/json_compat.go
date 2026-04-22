package tools

import (
	"encoding/json"
	"strings"
)

func normalizeStringifiedJSONFields(args json.RawMessage, fields ...string) (json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil, err
	}

	changed := false
	for _, field := range fields {
		value, ok := raw[field]
		if !ok {
			continue
		}
		normalized, ok := decodeStringifiedJSON(value)
		if !ok {
			continue
		}
		raw[field] = normalized
		changed = true
	}

	if !changed {
		return args, nil
	}
	return json.Marshal(raw)
}

func decodeStringifiedJSON(value json.RawMessage) (json.RawMessage, bool) {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return nil, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, false
	}
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		return nil, false
	}
	if !json.Valid([]byte(text)) {
		return nil, false
	}
	return json.RawMessage(text), true
}
