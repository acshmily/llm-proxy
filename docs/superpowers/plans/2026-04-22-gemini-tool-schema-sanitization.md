# Fix Gemini Tool Schema Compatibility

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Strip unsupported JSON Schema fields from tool definitions before sending to Gemini API, fixing the 400 Bad Request error when OpenClaw sends tools with `additionalProperties`, `$ref`, etc.

**Architecture:** Add a `sanitizeSchemaForGemini()` helper function that recursively walks the tool parameters schema and removes fields Gemini doesn't support. Call it in the Gemini converter's `Convert()` function before building `functionDeclarations`.

**Tech Stack:** Go 1.21+, standard library only

**Root Cause:** OpenAI-compatible clients (OpenClaw) send tool definitions with `additionalProperties: false`, `$ref`, `strict`, and other JSON Schema fields that Gemini's function calling API rejects with 400. LiteLLM handles this by recursively stripping unsupported fields — we need the same approach.

---

### Task 1: Add sanitizeSchemaForGemini helper with tests

**Files:**
- Create: `internal/protocol/gemini/schema_sanitize.go`
- Create: `internal/protocol/gemini/schema_sanitize_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/protocol/gemini/schema_sanitize_test.go`:

```go
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
				"type":      "object",
				"$ref":      "#/definitions/User",
				"$schema":   "http://json-schema.org/draft-07/schema#",
				"$id":       "user-schema",
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
								"type":      "string",
								"default":   "unknown",
								"$ref":      "#/definitions/City",
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/protocol/gemini/... -run TestSanitizeSchemaForGemini -v`
Expected: FAIL with "undefined: sanitizeSchemaForGemini"

- [ ] **Step 3: Write minimal implementation**

Create `internal/protocol/gemini/schema_sanitize.go`:

```go
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
		case map[string]any:
			result[k] = sanitizeSchemaForGemini(val)
		default:
			result[k] = v
		}
	}

	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/protocol/gemini/... -run TestSanitizeSchemaForGemini -v`
Expected: All 9 subtests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/protocol/gemini/schema_sanitize.go internal/protocol/gemini/schema_sanitize_test.go
git commit -m "feat: add sanitizeSchemaForGemini to strip unsupported JSON Schema fields"
```

---

### Task 2: Integrate schema sanitization into Convert

**Files:**
- Modify: `internal/protocol/gemini/converter.go`
- Modify: `internal/protocol/gemini/converter_test.go`

- [ ] **Step 1: Write the failing test — tools with additionalProperties**

Add to `internal/protocol/gemini/converter_test.go`:

```go
func TestConvert_SanitizesToolSchema(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "Get weather"},
		},
		Tools: []types.Tool{
			{
				Type: "function",
				Function: types.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters: map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":      "string",
								"default":   "unknown",
								"$ref":      "#/definitions/Location",
							},
						},
					},
				},
			},
		},
	}

	data, err := Convert(um, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	tools := result["tools"].([]interface{})
	tool := tools[0].(map[string]interface{})
	funcDecls := tool["functionDeclarations"].([]interface{})
	params := funcDecls[0].(map[string]interface{})["parameters"].(map[string]interface{})

	// Verify unsupported fields are removed
	if _, ok := params["additionalProperties"]; ok {
		t.Error("Expected additionalProperties to be removed")
	}

	location := params["properties"].(map[string]interface{})["location"].(map[string]interface{})
	if _, ok := location["default"]; ok {
		t.Error("Expected default to be removed")
	}
	if _, ok := location["$ref"]; ok {
		t.Error("Expected $ref to be removed")
	}

	// Verify valid fields are preserved
	if params["type"] != "object" {
		t.Errorf("Expected type 'object', got %v", params["type"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/protocol/gemini/... -run TestConvert_SanitizesToolSchema -v`
Expected: FAIL — additionalProperties and other fields are still present

- [ ] **Step 3: Modify Convert to call sanitizeSchemaForGemini**

In `internal/protocol/gemini/converter.go`, change the tools section (lines 39-51) from:

```go
// 添加 tools 支持
if len(um.Tools) > 0 {
	funcDeclarations := make([]map[string]interface{}, len(um.Tools))
	for i, tool := range um.Tools {
		funcDeclarations[i] = map[string]interface{}{
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"parameters":  tool.Function.Parameters,
		}
	}
	req["tools"] = []map[string]interface{}{
		{"functionDeclarations": funcDeclarations},
	}
}
```

To:

```go
// 添加 tools 支持
if len(um.Tools) > 0 {
	funcDeclarations := make([]map[string]interface{}, len(um.Tools))
	for i, tool := range um.Tools {
		funcDeclarations[i] = map[string]interface{}{
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
			"parameters":  sanitizeSchemaForGemini(tool.Function.Parameters),
		}
	}
	req["tools"] = []map[string]interface{}{
		{"functionDeclarations": funcDeclarations},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/protocol/gemini/... -run TestConvert_SanitizesToolSchema -v`
Expected: PASS

- [ ] **Step 5: Run all tests to verify no regression**

Run: `go test ./... -count=1`
Expected: All tests pass

- [ ] **Step 6: Commit**

```bash
git add internal/protocol/gemini/converter.go internal/protocol/gemini/converter_test.go
git commit -m "fix: sanitize tool schemas for Gemini compatibility in Convert"
```

---

### Task 3: Handle type assertion edge cases in sanitizeSchemaForGemini

**Files:**
- Modify: `internal/protocol/gemini/schema_sanitize.go`
- Modify: `internal/protocol/gemini/schema_sanitize_test.go`

- [ ] **Step 1: Write the failing test — array items and mixed types**

Add to `internal/protocol/gemini/schema_sanitize_test.go`:

```go
func TestSanitizeSchemaForGemini_ArrayItems(t *testing.T) {
	input := map[string]interface{}{
		"type": "array",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
		},
	}

	result := sanitizeSchemaForGemini(input)
	items := result["items"].(map[string]interface{})

	if _, ok := items["additionalProperties"]; ok {
		t.Error("Expected additionalProperties to be removed from items")
	}
	if items["type"] != "object" {
		t.Errorf("Expected items type 'object', got %v", items["type"])
	}
}

func TestSanitizeSchemaForGemini_AnyOfOneOf(t *testing.T) {
	// anyOf/oneOf are not supported by Gemini function calling
	// but we should not crash on them — just pass through as-is
	// (Gemini will handle gracefully or reject at API level)
	input := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"value": map[string]interface{}{
				"anyOf": []interface{}{
					map[string]interface{}{"type": "string"},
					map[string]interface{}{"type": "number"},
				},
			},
		},
	}

	result := sanitizeSchemaForGemini(input)
	// Should not panic, anyOf should be passed through (not in strip list)
	value := result["properties"].(map[string]interface{})["value"].(map[string]interface{})
	if _, ok := value["anyOf"]; !ok {
		t.Error("Expected anyOf to be preserved (not in unsupported list)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/protocol/gemini/... -run "TestSanitizeSchemaForGemini_ArrayItems|TestSanitizeSchemaForGemini_AnyOfOneOf" -v`
Expected: FAIL or panic due to type assertion on `map[string]any`

- [ ] **Step 3: Fix type assertions to handle Go map types**

The issue is that `json.Unmarshal` produces `map[string]interface{}`, but some code paths may produce `map[string]any`. Update `sanitizeSchemaForGemini` to handle both:

Modify the switch statement in `internal/protocol/gemini/schema_sanitize.go` from:

```go
switch val := v.(type) {
case map[string]interface{}:
	result[k] = sanitizeSchemaForGemini(val)
case map[string]any:
	result[k] = sanitizeSchemaForGemini(val)
default:
	result[k] = v
}
```

To use a type assertion approach that covers both:

```go
switch val := v.(type) {
case map[string]interface{}:
	result[k] = sanitizeSchemaForGemini(val)
default:
	result[k] = v
}
```

Note: `map[string]any` and `map[string]interface{}` are the same type in Go — no separate case needed. The test will pass once we verify this.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/protocol/gemini/... -v`
Expected: All tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/protocol/gemini/schema_sanitize.go internal/protocol/gemini/schema_sanitize_test.go
git commit -m "fix: handle array items and edge cases in schema sanitization"
```

---

## Self-Review

**1. Spec coverage:** All requirements met — sanitize function strips unsupported fields, integrated into Convert, tested with unit tests including edge cases.

**2. Placeholder scan:** No TBD/TODO/fill-in-later patterns found. All code steps contain actual implementation code.

**3. Type consistency:** `sanitizeSchemaForGemini` takes and returns `map[string]interface{}`, matching `FunctionDefinition.Parameters` type. Function signature is consistent across all tasks.
