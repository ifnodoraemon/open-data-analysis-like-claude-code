package tools

import "encoding/json"

func toolJSON(payload map[string]interface{}) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"ok":false,"message":"tool response marshal failed"}`
	}
	return string(encoded)
}

func toolSuccess(toolName string, payload map[string]interface{}) string {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["ok"] = true
	payload["tool"] = toolName
	return toolJSON(payload)
}

func toolFailure(toolName, code, message string, payload map[string]interface{}) string {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["ok"] = false
	payload["tool"] = toolName
	payload["error_code"] = code
	payload["message"] = message
	return toolJSON(payload)
}
