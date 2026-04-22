package gemini

import (
	"reflect"
	"testing"
)

func TestSanitizeSchemaForGemini(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "removes additionalProperties",
			input: map[string]interface{}{
				"type":                 "object",
				"properties":           map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
				"additionalProperties": false,
			},
			expected: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{"name": map[string]interface{}{"type": "string"}},
			},
		},
		{
			name: "removes $ref and $schema",
			input: map[string]interface{}{
				"type":        "object",
				"$ref":        "#/definitions/User",
				"$schema":     "http://json-schema.org/draft-07/schema#",
				"$id":         "user-schema",
				"description": "A user",
			},
			expected: map[string]interface{}{
				"type":        "object",
				"description": "A user",
			},
		},
		{
			name: "removes default and deprecated",
			input: map[string]interface{}{
				"type":       "string",
				"default":    "hello",
				"deprecated": true,
			},
			expected: map[string]interface{}{
				"type": "string",
			},
		},
		{
			name: "removes patternProperties and propertyNames",
			input: map[string]interface{}{
				"type":              "object",
				"patternProperties": map[string]interface{}{"^S_": map[string]interface{}{"type": "string"}},
				"propertyNames":     map[string]interface{}{"minLength": 1},
			},
			expected: map[string]interface{}{
				"type": "object",
			},
		},
		{
			name: "recursively sanitizes nested properties",
			input: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"address": map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"city": map[string]interface{}{
								"type":    "string",
								"default": "unknown",
								"$ref":    "#/definitions/City",
							},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"address": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"city": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
		{
			name: "sanitizes array items",
			input: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]interface{}{
						"name": map[string]interface{}{"type": "string"},
					},
				},
			},
			expected: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
		{
			name: "preserves valid schema",
			input: map[string]interface{}{
				"type":        "object",
				"description": "Test",
				"required":    []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
				},
			},
			expected: map[string]interface{}{
				"type":        "object",
				"description": "Test",
				"required":    []string{"name"},
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
				},
			},
		},
		{
			name:     "handles nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "handles empty object",
			input:    map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "sanitizes maps inside anyOf array",
			input: map[string]interface{}{
				"type": "string",
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "number", "additionalProperties": false},
				},
			},
			expected: map[string]interface{}{
				"type": "string",
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "number"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeSchemaForGemini(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("got:\n%v\nwant:\n%v", result, tt.expected)
			}
		})
	}
}
