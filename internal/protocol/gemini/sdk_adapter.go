package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/claude-projetc/llm-proxy/pkg/types"
	"google.golang.org/genai"
)

// UnifiedMessageToSDK 将统一消息转换为 SDK 参数
func UnifiedMessageToSDK(um *types.UnifiedMessage) (
	model string,
	contents []*genai.Content,
	systemInstruction *genai.Content,
	config *genai.GenerateContentConfig,
	tools []*genai.Tool,
	err error,
) {
	model = um.Model

	// 提取 system/developer 消息为 SystemInstruction
	contents, systemInstruction = buildContentsAndSystemInstruction(um.Messages)

	// 构建生成配置
	config = buildGenerateConfig(um)

	// 构建工具定义
	if len(um.Tools) > 0 {
		tools = buildTools(um.Tools)
		// 如果有 tool_choice，配置 ToolConfig
		if um.ToolChoice != nil {
			config.ToolConfig = buildToolConfig(um.ToolChoice)
		}
	}

	return
}

// buildContentsAndSystemInstruction 将消息转换为 SDK contents 和 system instruction
func buildContentsAndSystemInstruction(messages []types.MessageRole) ([]*genai.Content, *genai.Content) {
	var systemTexts []string
	var contents []*genai.Content

	for _, msg := range messages {
		switch msg.Role {
		case "system", "developer":
			systemTexts = append(systemTexts, msg.Content)
		case "user":
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: msg.Content}},
			})
		case "assistant":
			content := &genai.Content{Role: "model"}
			// 加入文本 part
			if msg.Content != "" {
				content.Parts = append(content.Parts, &genai.Part{Text: msg.Content})
			}
			// 加入 functionCall parts
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = map[string]any{}
				}
				part := &genai.Part{
					FunctionCall: &genai.FunctionCall{
						Name: tc.Function.Name,
						Args: args,
					},
				}
				if tc.ID != "" {
					part.FunctionCall.ID = tc.ID
				}
				content.Parts = append(content.Parts, part)
			}
			// 如果既没有文本也没有 tool_calls，跳过空消息
			if len(content.Parts) > 0 {
				contents = append(contents, content)
			}
		case "tool":
			// tool 响应映射为 user 角色的 FunctionResponse
			funcName := lookupFunctionNameFromToolCallID(msg.ToolCallID, messages)
			responseObj := parseToolResponse(msg.Content)

			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						Name:     funcName,
						Response: responseObj,
					},
				}},
			})
		default:
			contents = append(contents, &genai.Content{
				Role:  msg.Role,
				Parts: []*genai.Part{{Text: msg.Content}},
			})
		}
	}

	// 构建 SystemInstruction
	var sysInst *genai.Content
	if len(systemTexts) > 0 {
		sysInst = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{{Text: strings.Join(systemTexts, "\n\n")}},
		}
	}

	return contents, sysInst
}

// buildGenerateConfig 从 UnifiedMessage 构建 SDK 生成配置
func buildGenerateConfig(um *types.UnifiedMessage) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{}

	if um.MaxTokens > 0 {
		config.MaxOutputTokens = int32(um.MaxTokens)
	}
	if um.Temperature > 0 {
		temp := float32(um.Temperature)
		config.Temperature = &temp
	}
	if um.TopP > 0 {
		topP := float32(um.TopP)
		config.TopP = &topP
	}
	if len(um.StopSequences) > 0 {
		config.StopSequences = um.StopSequences
	}

	return config
}

// buildTools 将 Tool 定义转换为 SDK Tool
func buildTools(tools []types.Tool) []*genai.Tool {
	sdkTools := make([]*genai.Tool, len(tools))
	for i, tool := range tools {
		decl := &genai.FunctionDeclaration{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
		}
		if params := sanitizeSchemaForGemini(tool.Function.Parameters); params != nil {
			// 将 map 转换为 *genai.Schema
			if data, err := json.Marshal(params); err == nil {
				var schema *genai.Schema
				if err := json.Unmarshal(data, &schema); err == nil {
					decl.Parameters = schema
				}
			}
		}
		sdkTools[i] = &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{decl},
		}
	}
	return sdkTools
}

// buildToolConfig 从 ToolChoice 构建 ToolConfig
func buildToolConfig(toolChoice interface{}) *genai.ToolConfig {
	switch v := toolChoice.(type) {
	case string:
		var mode genai.FunctionCallingConfigMode
		switch v {
		case "none":
			mode = genai.FunctionCallingConfigModeNone
		case "required", "any":
			mode = genai.FunctionCallingConfigModeAny
		default: // "auto"
			mode = genai.FunctionCallingConfigModeAuto
		}
		return &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: mode},
		}
	case map[string]interface{}:
		if fn, ok := v["function"].(map[string]interface{}); ok {
			if name, ok := fn["name"].(string); ok {
				return &genai.ToolConfig{
					FunctionCallingConfig: &genai.FunctionCallingConfig{
						Mode:                 genai.FunctionCallingConfigModeAny,
						AllowedFunctionNames: []string{name},
					},
				}
			}
		}
	}
	return nil
}

func lookupFunctionNameFromToolCallID(toolCallID string, messages []types.MessageRole) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			for _, tc := range messages[i].ToolCalls {
				if tc.ID == toolCallID {
					return tc.Function.Name
				}
			}
		}
	}
	return toolCallID
}

func parseToolResponse(content string) map[string]interface{} {
	var resp map[string]interface{}
	if content == "" {
		return map[string]interface{}{}
	}
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return map[string]interface{}{"output": content}
	}
	return resp
}

// FromSDKResponse 将 SDK 响应转换为统一响应格式
func FromSDKResponse(resp *genai.GenerateContentResponse, model string) (*types.UnifiedResponse, error) {
	var content []types.ContentBlock
	var toolCalls []types.ToolCall
	var finishReason string

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		if candidate.Content != nil {
			for i, part := range candidate.Content.Parts {
				if part.FunctionCall != nil {
					argsJSON := "{}"
					if part.FunctionCall.Args != nil {
						if b, err := json.Marshal(part.FunctionCall.Args); err == nil {
							argsJSON = string(b)
						}
					}
					toolCalls = append(toolCalls, types.ToolCall{
						ID:   fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, i),
						Type: "function",
						Function: types.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: argsJSON,
						},
					})
				} else if part.Text != "" {
					content = append(content, types.ContentBlock{
						Type: "text",
						Text: part.Text,
					})
				}
			}
		}
		finishReason = mapSDKFinishReason(candidate.FinishReason)
	}

	usage := types.Usage{}
	if resp.UsageMetadata != nil {
		usage.InputTokens = int(resp.UsageMetadata.PromptTokenCount)
		usage.OutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return &types.UnifiedResponse{
		ID:           fmt.Sprintf("gemini-sdk-%d", len(resp.Candidates)),
		Model:        model,
		Content:      content,
		Role:         "assistant",
		FinishReason: finishReason,
		ToolCalls:    toolCalls,
		Usage:        usage,
	}, nil
}

func mapSDKFinishReason(reason genai.FinishReason) string {
	switch reason {
	case genai.FinishReasonStop:
		return "stop"
	case genai.FinishReasonMaxTokens:
		return "length"
	case genai.FinishReasonSafety:
		return "content_filter"
	default:
		return "stop"
	}
}

// SDKResponseToREST 将 SDK 响应转换为 Gemini REST API JSON 格式（用于原生端点兼容）
func SDKResponseToREST(resp *genai.GenerateContentResponse) ([]byte, error) {
	result := map[string]interface{}{
		"candidates": []interface{}{},
	}

	for _, candidate := range resp.Candidates {
		candMap := map[string]interface{}{
			"finishReason": string(candidate.FinishReason),
		}

		if candidate.Content != nil {
			parts := []interface{}{}
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					parts = append(parts, map[string]interface{}{"text": part.Text})
				}
				if part.FunctionCall != nil {
					parts = append(parts, map[string]interface{}{
						"functionCall": map[string]interface{}{
							"name":      part.FunctionCall.Name,
							"arguments": part.FunctionCall.Args,
						},
					})
				}
			}
			candMap["content"] = map[string]interface{}{
				"parts": parts,
				"role":  candidate.Content.Role,
			}
		}

		result["candidates"] = append(result["candidates"].([]interface{}), candMap)
	}

	if resp.UsageMetadata != nil {
		result["usageMetadata"] = map[string]interface{}{
			"promptTokenCount":     resp.UsageMetadata.PromptTokenCount,
			"candidatesTokenCount": resp.UsageMetadata.CandidatesTokenCount,
			"totalTokenCount":      resp.UsageMetadata.TotalTokenCount,
		}
	}

	return json.Marshal(result)
}
