// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	geniex_sdk "github.com/qualcomm/GenieX/bindings/go"
)

func TestMinistral3ChatResponses(t *testing.T) {
	const call = `[TOOL_CALLS]get_weather[ARGS]{"city":"Paris"}`
	cases := []struct {
		name               string
		tokens             []string
		inline             bool
		content, reasoning string
		calls              []openai.ChatCompletionMessageFunctionToolCallFunction
	}{
		{
			name: "single call", tokens: []string{"[TOOL_CALLS]", "get_", "weather", "[ARGS]", `{"city":`, `"Paris"}`, " done"},
			content: " done", calls: []openai.ChatCompletionMessageFunctionToolCallFunction{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name: "inline call", inline: true, tokens: []string{call},
			calls: []openai.ChatCompletionMessageFunctionToolCallFunction{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name: "parallel calls with reasoning", tokens: []string{"[THINK]", "check weather and time", "[/THINK]", "sure ", call, "[TOOL_CALLS]get_time[ARGS]{}"},
			content: "sure ", reasoning: "check weather and time",
			calls: []openai.ChatCompletionMessageFunctionToolCallFunction{{Name: "get_weather", Arguments: `{"city":"Paris"}`}, {Name: "get_time", Arguments: `{}`}},
		},
		{
			name: "fake call inside reasoning", tokens: []string{"[THINK]", "example: ", "[TOOL_CALLS]", "fake", "[ARGS]", "{}", "[/THINK]", call},
			reasoning: "example: [TOOL_CALLS]fake[ARGS]{}",
			calls:     []openai.ChatCompletionMessageFunctionToolCallFunction{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name: "inline reasoning stays inline", inline: true, tokens: []string{"[THINK]", "check weather", "[/THINK]", call},
			content: "[THINK]check weather[/THINK]",
			calls:   []openai.ChatCompletionMessageFunctionToolCallFunction{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name: "reasoning without calls", tokens: []string{"[THINK]", "reason", "[/THINK]", "answer"},
			content: "answer", reasoning: "reason",
		},
		{
			name: "empty reasoning", tokens: []string{"[THINK]", "[/THINK]", call},
			calls: []openai.ChatCompletionMessageFunctionToolCallFunction{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name: "unterminated reasoning", tokens: []string{"[THINK]", "example: ", call},
			reasoning: "example: " + call,
		},
		{
			name: "truncated arguments", tokens: []string{"[THINK]", "reason", "[/THINK]", `[TOOL_CALLS]get_weather[ARGS]{"city":`},
			content: `[TOOL_CALLS]get_weather[ARGS]{"city":`, reasoning: "reason",
		},
		{
			name: "invalid arguments", tokens: []string{`[TOOL_CALLS]bad[ARGS]{"x":}`, call},
			content: `[TOOL_CALLS]bad[ARGS]{"x":}`,
			calls:   []openai.ChatCompletionMessageFunctionToolCallFunction{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class := func() tokenClass {
				if tc.inline {
					return plainClass
				}
				return reasoningClass()
			}
			wantFinish := "stop"
			if len(tc.calls) > 0 {
				wantFinish = "tool_calls"
			}
			var content, reasoning strings.Builder
			onToken, finish := sinkWithToolCalls(class(), &content, &reasoning, "")
			for _, token := range tc.tokens {
				onToken(token)
			}
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			writeBlockingResponse(c, &content, &reasoning, geniex_sdk.ProfileData{StopReason: "eos"}, finish, "test-model")
			var body blockingBody
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || len(body.Choices) != 1 {
				t.Fatalf("status=%d, choices=%+v", w.Code, body.Choices)
			}
			choice := body.Choices[0]
			if choice.Message.Content != tc.content || choice.Message.ReasoningContent != tc.reasoning || choice.FinishReason != wantFinish {
				t.Errorf("blocking choice=%+v", choice)
			}
			if len(choice.Message.ToolCalls) != len(tc.calls) {
				t.Fatalf("blocking calls=%+v, want %+v", choice.Message.ToolCalls, tc.calls)
			}
			for i, call := range choice.Message.ToolCalls {
				if call.Type != "function" || !strings.HasPrefix(call.ID, "call_") || call.Function.Name != tc.calls[i].Name || call.Function.Arguments != tc.calls[i].Arguments {
					t.Errorf("blocking call=%+v, want %+v", call, tc.calls[i])
				}
			}

			deltas := runStreamToolCallWithTools(t, class(), "", tc.tokens...)
			var streamedContent, streamedReasoning strings.Builder
			var calls []toolCallDelta
			for i, delta := range deltas {
				if i < len(deltas)-1 && delta.FinishReason != nil {
					t.Errorf("intermediate finish_reason=%v, want null", delta.FinishReason)
				}
				streamedContent.WriteString(delta.Delta.Content)
				streamedReasoning.WriteString(delta.Delta.ReasoningContent)
				for _, call := range delta.Delta.ToolCalls {
					if call.Type != "function" || !strings.HasPrefix(call.ID, "call_") {
						t.Errorf("streamed metadata=%+v", call)
					}
					calls = append(calls, toolCallDelta{int(call.Index), call.Function.Name, call.Function.Arguments})
				}
			}
			if streamedContent.String() != tc.content || streamedReasoning.String() != tc.reasoning || len(calls) != len(tc.calls) {
				t.Errorf("streamed=(%q, %q, %+v)", streamedContent.String(), streamedReasoning.String(), calls)
			}
			for i, call := range calls {
				if i < len(tc.calls) && (call.index != i || call.name != tc.calls[i].Name || call.arguments != tc.calls[i].Arguments) {
					t.Errorf("streamed call=%+v, want %+v", call, tc.calls[i])
				}
			}
			if len(deltas) == 0 {
				t.Fatal("stream omitted terminal chunk")
			}
			if reason := deltas[len(deltas)-1].FinishReason; reason == nil || *reason != wantFinish {
				t.Errorf("streamed finish_reason=%v, want %q", reason, wantFinish)
			}
		})
	}
}
