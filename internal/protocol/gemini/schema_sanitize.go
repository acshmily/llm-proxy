package gemini

// unsupportedSchemaFields lists JSON Schema keys that Gemini function calling
// does not support. See: https://ai.google.dev/gemini-api/docs/function-calling
var unsupportedSchemaFields = []string{
	"additionalProperties",
	"patternProperties",
	"propertyNames",
	"$ref",
	"$schema",
	"$id",
	"default",
	"deprecated",
}

// sanitizeSchemaForGemini recursively removes unsupported JSON Schema fields
// from tool parameter definitions before sending to Gemini API.
func sanitizeSchemaForGemini(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := make(map[string]interface{}, len(schema))
	for k, v := range schema {
		// Skip unsupported fields
		skip := false
		for _, field := range unsupportedSchemaFields {
			if k == field {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		// Recursively sanitize nested maps
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = sanitizeSchemaForGemini(val)
		case []interface{}:
			sanitized := make([]interface{}, len(val))
			for i, elem := range val {
				if m, ok := elem.(map[string]interface{}); ok {
					sanitized[i] = sanitizeSchemaForGemini(m)
				} else {
					sanitized[i] = elem
				}
			}
			result[k] = sanitized
		default:
			result[k] = v
		}
	}

	return result
}
