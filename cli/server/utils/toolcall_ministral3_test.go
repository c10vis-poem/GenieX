// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package utils

import "testing"

func TestMinistral3ToolCallScanner(t *testing.T) {
	type scannerCase struct {
		name, response, text string
		calls                []toolCallFn
	}
	cases := []scannerCase{
		{
			name: "single call", response: `[TOOL_CALLS]get_weather[ARGS]{"city":"Paris"}`,
			calls: []toolCallFn{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name: "empty arguments", response: `[TOOL_CALLS]f[ARGS]{}`,
			calls: []toolCallFn{{Name: "f", Arguments: `{}`}},
		},
		{
			name: "parallel calls", response: `[TOOL_CALLS]f[ARGS]{}[TOOL_CALLS]g[ARGS]{"x":1}`,
			calls: []toolCallFn{{Name: "f", Arguments: `{}`}, {Name: "g", Arguments: `{"x":1}`}},
		},
		{
			name: "surrounding text", response: `before [TOOL_CALLS]f[ARGS]{} between [TOOL_CALLS]g[ARGS]{} after`, text: "before  between  after",
			calls: []toolCallFn{{Name: "f", Arguments: `{}`}, {Name: "g", Arguments: `{}`}},
		},
		{
			name: "nested arguments", response: `[TOOL_CALLS]batch[ARGS]{"items":[{"data":{"ok":true}},null],"n":1.5}`,
			calls: []toolCallFn{{Name: "batch", Arguments: `{"items":[{"data":{"ok":true}},null],"n":1.5}`}},
		},
		{
			name: "braces escapes and markers inside strings", response: `[TOOL_CALLS]f[ARGS]{"text":"}{ \" [TOOL_CALLS]fake[ARGS]{}","path":"C:\\"}`,
			calls: []toolCallFn{{Name: "f", Arguments: `{"text":"}{ \" [TOOL_CALLS]fake[ARGS]{}","path":"C:\\"}`}},
		},
		{
			name: "arguments resembling a generic call", response: `[TOOL_CALLS]outer[ARGS]{"name":"inner","arguments":{}}`,
			calls: []toolCallFn{{Name: "outer", Arguments: `{"name":"inner","arguments":{}}`}},
		},
		{
			name: "whitespace before arguments", response: "[TOOL_CALLS]f[ARGS]\n\t {\"x\": 1}",
			calls: []toolCallFn{{Name: "f", Arguments: `{"x": 1}`}},
		},
		{
			name: "marker quoted in a bare JSON call", response: `{"name":"outer","arguments":{"text":"[TOOL_CALLS]fake[ARGS]{}"}}`,
			calls: []toolCallFn{{Name: "outer", Arguments: `{"text":"[TOOL_CALLS]fake[ARGS]{}"}`}},
		},
		{
			name: "marker quoted in a Qwen call", response: `<tool_call>{"name":"outer","arguments":{"text":"quote \\\" [TOOL_CALLS]fake[ARGS]{}"}}</tool_call>`,
			calls: []toolCallFn{{Name: "outer", Arguments: `{"text":"quote \\\" [TOOL_CALLS]fake[ARGS]{}"}`}},
		},
		{
			name: "marker quoted in ordinary JSON", response: `{"text":"[TOOL_CALLS]fake[ARGS]{}"}`, text: `{"text":"[TOOL_CALLS]fake[ARGS]{}"}`,
		},
		{
			name: "mixed formats", response: `[TOOL_CALLS]f[ARGS]{}<tool_call>{"name":"g","arguments":{}}</tool_call>`,
			calls: []toolCallFn{{Name: "f", Arguments: `{}`}, {Name: "g", Arguments: `{}`}},
		},
		{
			name: "invalid JSON then valid call", response: `[TOOL_CALLS]bad[ARGS]{"x":}[TOOL_CALLS]good[ARGS]{}`, text: `[TOOL_CALLS]bad[ARGS]{"x":}`,
			calls: []toolCallFn{{Name: "good", Arguments: `{}`}},
		},
		{
			name: "abandoned header then valid call", response: `[TOOL_CALLS]bad[TOOL_CALLS]good[ARGS]{}`, text: `[TOOL_CALLS]bad`,
			calls: []toolCallFn{{Name: "good", Arguments: `{}`}},
		},
		{
			name: "empty header then valid call", response: `[TOOL_CALLS][TOOL_CALLS]good[ARGS]{}`, text: `[TOOL_CALLS]`,
			calls: []toolCallFn{{Name: "good", Arguments: `{}`}},
		},
	}
	for _, response := range []string{
		"plain text", "[TOOL_C", "[TOOL_CALLS]", "[TOOL_CALLS]f", "[TOOL_CALLS]f[AR",
		"[TOOL_CALLS]f[ARGS]", "[TOOL_CALLS]f[ARGS]  ", `[TOOL_CALLS]f[ARGS]{"x":`,
		`[TOOL_CALLS]f[ARGS]{"x":}`, `[TOOL_CALLS][ARGS]{}`, `[TOOL_CALLS]f{}`,
		`[TOOL_CALLS]f[ARGS]null`, `[TOOL_CALLS]f[ARGS][1,2]`, `[TOOL_CALLS]not a function[ARGS]{}`,
		`[TOOL_CALLS]f[CALL_ID]123456789[ARGS]{}`, `[TOOL_CALLX]f[ARGS]{}`,
	} {
		cases = append(cases, scannerCase{name: "literal/" + response, response: response, text: response})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkToolCallScanner(t, tc.response, tc.text, tc.calls, nil)
		})
	}
}

func TestMinistral3ToolCallEmitsOnClosingBrace(t *testing.T) {
	scanner := NewToolCallScanner(nil)
	for _, chunk := range []string{"[TOOL_", "CALLS]", "get_", "weather", "[AR", "GS]", `{"city":`, `"Paris"`} {
		if text, calls := scanner.Push(chunk); text != "" || len(calls) != 0 {
			t.Fatalf("Push(%q)=(%q, %+v), want held candidate", chunk, text, calls)
		}
	}
	want := []toolCallFn{{Name: "get_weather", Arguments: `{"city":"Paris"}`}}
	if text, calls := scanner.Push("}"); text != "" || !callsEqual(calls, want) {
		t.Fatalf("closing brace=(%q, %+v), want %+v", text, calls, want)
	}
	if text, calls := scanner.Push(" done"); text != " done" || len(calls) != 0 {
		t.Fatalf("following text=(%q, %+v)", text, calls)
	}
	if text, calls := scanner.Tail(); text != "" || len(calls) != 0 {
		t.Fatalf("Tail=(%q, %+v), want empty", text, calls)
	}
}
