package gemini

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/claude-projetc/llm-proxy/pkg/types"
	"google.golang.org/genai"
)

func TestUnifiedMessageToSDK_BasicMessage(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.0-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "Hello"},
		},
		Stream: false,
	}

	model, contents, sysInst, config, tools, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if model != "gemini-2.0-flash" {
		t.Errorf("Expected model 'gemini-2.0-flash', got %q", model)
	}
	if len(contents) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(contents))
	}
	if contents[0].Role != "user" {
		t.Errorf("Expected content role 'user', got %q", contents[0].Role)
	}
	if sysInst != nil {
		t.Error("Expected nil SystemInstruction for non-system messages")
	}
	if config == nil {
		t.Fatal("Expected non-nil config")
	}
	if len(tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(tools))
	}
}

func TestUnifiedMessageToSDK_SystemInstruction(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.0-flash",
		Messages: []types.MessageRole{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	}

	model, contents, sysInst, _, _, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if sysInst == nil {
		t.Fatal("Expected non-nil SystemInstruction")
	}
	if len(sysInst.Parts) != 1 {
		t.Fatalf("Expected 1 part in SystemInstruction, got %d", len(sysInst.Parts))
	}
	if sysInst.Parts[0].Text != "You are helpful." {
		t.Errorf("Expected SystemInstruction text 'You are helpful.', got %q", sysInst.Parts[0].Text)
	}
	// system 消息不应出现在 contents 中
	if len(contents) != 1 {
		t.Errorf("Expected 1 content (user only), got %d", len(contents))
	}
	if contents[0].Role != "user" {
		t.Errorf("Expected content role 'user', got %q", contents[0].Role)
	}
	_ = model
}

func TestUnifiedMessageToSDK_AllParameters(t *testing.T) {
	um := &types.UnifiedMessage{
		Model:         "gemini-2.0-flash",
		Messages:      []types.MessageRole{{Role: "user", Content: "Hello"}},
		MaxTokens:     100,
		Temperature:   0.7,
		TopP:          0.9,
		StopSequences: []string{"\n", "END"},
		Stream:        false,
	}

	_, _, _, config, _, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if config.MaxOutputTokens != 100 {
		t.Errorf("Expected MaxOutputTokens 100, got %d", config.MaxOutputTokens)
	}
	if config.Temperature == nil || *config.Temperature != 0.7 {
		t.Errorf("Expected Temperature 0.7, got %v", config.Temperature)
	}
	if config.TopP == nil || *config.TopP != 0.9 {
		t.Errorf("Expected TopP 0.9, got %v", config.TopP)
	}
	if config.StopSequences == nil || !reflect.DeepEqual(config.StopSequences, []string{"\n", "END"}) {
		t.Errorf("Expected StopSequences ['\\n','END'], got %v", config.StopSequences)
	}
}

func TestUnifiedMessageToSDK_ToolChoice(t *testing.T) {
	um := &types.UnifiedMessage{
		Model:    "gemini-2.0-flash",
		Messages: []types.MessageRole{{Role: "user", Content: "Hello"}},
		Tools: []types.Tool{{
			Type: "function",
			Function: types.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get weather",
			},
		}},
		ToolChoice: "auto",
	}

	_, _, _, config, tools, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}
	if config == nil {
		t.Fatal("Expected non-nil config")
	}
	if config.ToolConfig == nil {
		t.Fatal("Expected non-nil ToolConfig for tool_choice")
	}
}

func TestUnifiedMessageToSDK_ToolChoiceNone(t *testing.T) {
	um := &types.UnifiedMessage{
		Model:      "gemini-2.0-flash",
		Messages:   []types.MessageRole{{Role: "user", Content: "Hello"}},
		Tools:      []types.Tool{{Type: "function", Function: types.FunctionDefinition{Name: "test"}}},
		ToolChoice: "none",
	}

	_, _, _, config, _, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if config.ToolConfig == nil {
		t.Fatal("Expected ToolConfig for tool_choice=none")
	}
	if config.ToolConfig.FunctionCallingConfig.Mode != genai.FunctionCallingConfigModeNone {
		t.Errorf("Expected FunctionCallingConfigModeNone, got %s", config.ToolConfig.FunctionCallingConfig.Mode)
	}
}

func TestUnifiedMessageToSDK_ToolChoiceSpecificFunction(t *testing.T) {
	um := &types.UnifiedMessage{
		Model:      "gemini-2.0-flash",
		Messages:   []types.MessageRole{{Role: "user", Content: "Hello"}},
		Tools:      []types.Tool{{Type: "function", Function: types.FunctionDefinition{Name: "get_weather"}}},
		ToolChoice: map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "get_weather"}},
	}

	_, _, _, config, _, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if config.ToolConfig == nil {
		t.Fatal("Expected ToolConfig for specific function tool_choice")
	}
	fc := config.ToolConfig.FunctionCallingConfig
	if fc.Mode != genai.FunctionCallingConfigModeAny {
		t.Errorf("Expected Mode ANY, got %s", fc.Mode)
	}
	if len(fc.AllowedFunctionNames) != 1 || fc.AllowedFunctionNames[0] != "get_weather" {
		t.Errorf("Expected AllowedFunctionNames ['get_weather'], got %v", fc.AllowedFunctionNames)
	}
}

func TestFromSDKResponse_BasicText(t *testing.T) {
	sdkResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{{Text: "Hello, world!"}},
			},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
		},
	}

	unified, err := FromSDKResponse(sdkResp, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("FromSDKResponse failed: %v", err)
	}

	if len(unified.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(unified.Content))
	}
	if unified.Content[0].Text != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got %q", unified.Content[0].Text)
	}
	if unified.Model != "gemini-2.0-flash" {
		t.Errorf("Expected model 'gemini-2.0-flash', got %q", unified.Model)
	}
	if unified.FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got %q", unified.FinishReason)
	}
	if unified.Usage.InputTokens != 10 {
		t.Errorf("Expected 10 input tokens, got %d", unified.Usage.InputTokens)
	}
	if unified.Usage.OutputTokens != 5 {
		t.Errorf("Expected 5 output tokens, got %d", unified.Usage.OutputTokens)
	}
}

func TestFromSDKResponse_FunctionCall(t *testing.T) {
	args := map[string]any{"location": "Tokyo"}
	sdkResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{{
					FunctionCall: &genai.FunctionCall{
						Name: "get_weather",
						Args: args,
					},
				}},
			},
			FinishReason: genai.FinishReasonStop,
		}},
	}

	unified, err := FromSDKResponse(sdkResp, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("FromSDKResponse failed: %v", err)
	}

	if len(unified.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call, got %d", len(unified.ToolCalls))
	}
	if unified.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got %q", unified.ToolCalls[0].Function.Name)
	}
	// 验证 arguments 是有效的 JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(unified.ToolCalls[0].Function.Arguments), &parsed); err != nil {
		t.Fatalf("ToolCall arguments is not valid JSON: %v", err)
	}
	if parsed["location"] != "Tokyo" {
		t.Errorf("Expected argument location='Tokyo', got %v", parsed["location"])
	}
}

func TestSDKResponseToREST(t *testing.T) {
	sdkResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{{Text: "Hello"}},
			},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
			TotalTokenCount:      15,
		},
	}

	data, err := SDKResponseToREST(sdkResp)
	if err != nil {
		t.Fatalf("SDKResponseToREST failed: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	candidates, ok := resp["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		t.Fatal("Expected candidates array")
	}

	cand := candidates[0].(map[string]interface{})
	content := cand["content"].(map[string]interface{})
	parts := content["parts"].([]interface{})
	if len(parts) != 1 {
		t.Fatalf("Expected 1 part, got %d", len(parts))
	}

	part := parts[0].(map[string]interface{})
	if part["text"] != "Hello" {
		t.Errorf("Expected text 'Hello', got %v", part["text"])
	}

	usage := resp["usageMetadata"].(map[string]interface{})
	if int(usage["promptTokenCount"].(float64)) != 10 {
		t.Errorf("Expected promptTokenCount 10, got %v", usage["promptTokenCount"])
	}
}
