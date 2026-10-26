// Copyright (c) 2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	geniex_sdk "github.com/qualcomm/GenieX/bindings/go"
	"github.com/qualcomm/GenieX/cli/internal/thinkfsm"
)

func sseDeltas(t *testing.T, body string) []streamChoice {
	t.Helper()
	var out []streamChoice
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data:")
		if !ok || strings.TrimSpace(data) == "[DONE]" {
			continue
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("chunk %s: %v", data, err)
		}
		if chunk.Object != streamChunkObject {
			t.Errorf("object=%q, want %q", chunk.Object, streamChunkObject)
		}
		out = append(out, chunk.Choices...)
	}
	return out
}

type streamRecorder struct {
	*httptest.ResponseRecorder
	gone chan bool
}

func (r *streamRecorder) CloseNotify() <-chan bool { return r.gone }

func runStreamToolCallWithTools(t *testing.T, class tokenClass, tools string, tokens ...string) []streamChoice {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := &streamRecorder{httptest.NewRecorder(), make(chan bool)}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	dataCh := make(chan string, len(tokens))
	for _, tok := range tokens {
		dataCh <- tok
	}
	close(dataCh)
	profile := geniex_sdk.ProfileData{StopReason: "eos"}
	streamToolCall(c, dataCh, func() error { return nil }, false, &profile, class, tools)
	if !strings.Contains(w.Body.String(), "data:[DONE]") {
		t.Error("stream omitted [DONE]")
	}
	return sseDeltas(t, w.Body.String())
}

type toolCallDelta struct {
	index     int
	name      string
	arguments string
}

func TestStreamToolCallResponse(t *testing.T) {
	const call = `{"name":"f","arguments":{"x":1}}`
	const tools = `[{"type":"function","function":{"name":"set","parameters":{"type":"object","properties":{"count":{"type":"integer"},"label":{"type":"string"}}}}}]`
	cases := []struct {
		name               string
		events             []thinkfsm.Event
		tools              string
		content, reasoning string
		calls              []toolCallDelta
	}{
		{
			name:    "text without calls",
			events:  []thinkfsm.Event{{Kind: thinkfsm.Content, Text: "answer"}},
			content: "answer",
		},
		{
			name: "display fields and internal events",
			events: []thinkfsm.Event{
				{Kind: thinkfsm.Consume}, {Kind: thinkfsm.EnterThinking},
				{Kind: thinkfsm.Thinking, Text: call}, {Kind: thinkfsm.LeaveThinking},
				{Kind: thinkfsm.Content, Text: "sure "}, {Kind: thinkfsm.Content, Text: call},
				{Kind: thinkfsm.Content, Text: " done"},
			},
			content: "sure  done", reasoning: call,
			calls: []toolCallDelta{{0, "f", `{"x":1}`}},
		},
		{
			name: "parallel calls keep rising indexes",
			events: []thinkfsm.Event{
				{Kind: thinkfsm.Content, Text: `{"name":"a","arguments":{}}`},
				{Kind: thinkfsm.Content, Text: `{"name":"b","arguments":{}}`},
			},
			calls: []toolCallDelta{{0, "a", `{}`}, {1, "b", `{}`}},
		},
		{
			name: "MiniCPM5 schema reaches parser", tools: tools,
			events: []thinkfsm.Event{{Kind: thinkfsm.Content, Text: `<function name="set"><param name="count">5</param><param name="label">5</param></function>`}},
			calls:  []toolCallDelta{{0, "set", `{"count":5,"label":"5"}`}},
		},
		{
			name: "Spark2.5 schema reaches parser", tools: tools,
			events: []thinkfsm.Event{{Kind: thinkfsm.Content, Text: `<tool_call>set<arg_key>count</arg_key><arg_value>5</arg_value><arg_key>label</arg_key><arg_value>5</arg_value></tool_call>`}},
			calls:  []toolCallDelta{{0, "set", `{"count":5,"label":"5"}`}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := 0
			class := tokenClass(func(string) thinkfsm.Event {
				ev := tc.events[next]
				next++
				return ev
			})
			got := runStreamToolCallWithTools(t, class, tc.tools, make([]string, len(tc.events))...)
			var content, reasoning strings.Builder
			var calls []toolCallDelta
			for i, ch := range got {
				if i < len(got)-1 && ch.FinishReason != nil {
					t.Errorf("intermediate finish_reason=%v, want null", ch.FinishReason)
				}
				if (ch.Delta.Content != "" || ch.Delta.ReasoningContent != "") && ch.Delta.Role != "assistant" {
					t.Errorf("display role=%q, want assistant", ch.Delta.Role)
				}
				content.WriteString(ch.Delta.Content)
				reasoning.WriteString(ch.Delta.ReasoningContent)
				for _, call := range ch.Delta.ToolCalls {
					if call.Type != "function" || !strings.HasPrefix(call.ID, "call_") {
						t.Errorf("tool call metadata=%+v", call)
					}
					calls = append(calls, toolCallDelta{int(call.Index), call.Function.Name, call.Function.Arguments})
				}
			}
			if content.String() != tc.content || reasoning.String() != tc.reasoning {
				t.Errorf("display=(%q, %q), want (%q, %q)", content.String(), reasoning.String(), tc.content, tc.reasoning)
			}
			if len(calls) != len(tc.calls) {
				t.Fatalf("calls=%+v, want %+v", calls, tc.calls)
			}
			for i, call := range calls {
				if call != tc.calls[i] {
					t.Errorf("call %d=%+v, want %+v", i, call, tc.calls[i])
				}
			}
			if len(got) == 0 {
				t.Fatal("stream emitted no terminal chunk")
			}
			wantFinish := "stop"
			if len(tc.calls) > 0 {
				wantFinish = "tool_calls"
			}
			if reason := got[len(got)-1].FinishReason; reason == nil || *reason != wantFinish {
				t.Errorf("finish_reason=%v, want %q", reason, wantFinish)
			}
		})
	}
}
