// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package utils

import (
	"testing"

	"github.com/qualcomm/GenieX/cli/internal/thinkfsm"
)

func TestToolCallParserTokens(t *testing.T) {
	type tokenCase struct {
		name               string
		plain              bool
		tokens             []string
		content, reasoning string
		calls              []toolCallFn
	}
	cases := []tokenCase{
		{
			name: "plain keeps markers inline", plain: true,
			tokens:  []string{"<think>", "reason", "</think>", "<|channel|>", "analysis", "<|message|>", "answer", "<|end|>"},
			content: "<think>reason</think><|channel|>analysis<|message|>answer<|end|>",
		},
		{
			name: "Ministral3 inline reasoning", plain: true,
			tokens:  []string{"[THINK]", "reason", "[/THINK]", "[TOOL_CALLS]f[ARGS]{}"},
			content: "[THINK]reason[/THINK]", calls: []toolCallFn{{Name: "f", Arguments: `{}`}},
		},
		{
			name:      "Ministral3 reasoning with a literal call",
			tokens:    []string{"[THINK]", "[TOOL_CALLS]", "fake", "[ARGS]", "{}", "[/THINK]", "[TOOL_CALLS]", "f", "[ARGS]", "{}"},
			reasoning: "[TOOL_CALLS]fake[ARGS]{}", calls: []toolCallFn{{Name: "f", Arguments: `{}`}},
		},
		{
			name:    "separated think transition",
			tokens:  []string{"<think>", "reason", "</think>", "answer"},
			content: "answer", reasoning: "reason",
		},
		{
			name:    "ordinary commentary preamble",
			tokens:  []string{"<|start|>", "assistant", "<|channel|>", "comment", "ary", "<|message|>", "Let me check.", "<|call|>"},
			content: "Let me check.",
		},
		{
			name:   "commentary recipient tokens",
			tokens: []string{"<|channel|>", "comment", "ary", " to", "=", "functions", ".get", "_weather", "<|message|>", `{"city":"Paris"}`, "<|call|>"},
			calls:  []toolCallFn{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name:   "analysis role recipient",
			tokens: []string{"<|start|>", "assistant", " to", "=", "functions", ".get", "_weather", "<|channel|>", "analysis", "<|message|>", `{"city":"Paris"}`, "<|end|>"},
			calls:  []toolCallFn{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name:   "constrained commentary call at EOF",
			tokens: []string{"<|channel|>", "comment", "ary", " to", "=", "functions", ".", "get_open_tabs", "<|constrain|>", "json", "<|message|>", `{"topic":"tabs"}`},
			calls:  []toolCallFn{{Name: "get_open_tabs", Arguments: `{"topic":"tabs"}`}},
		},
		{
			name:      "role recipient after analysis",
			tokens:    []string{"<|channel|>", "analysis", "<|message|>", "thinking", "<|end|>", "<|start|>", "assistant", " to", "=", "functions", ".", "get_open_tabs", "<|channel|>", "comment", "ary", "<|message|>", `{"topic":"tabs"}`, "<|call|>"},
			reasoning: "thinking", calls: []toolCallFn{{Name: "get_open_tabs", Arguments: `{"topic":"tabs"}`}},
		},
		{
			name:      "recipient syntax inside analysis body stays literal",
			tokens:    []string{"<|channel|>", "analysis", "<|message|>", "I should check ", " to=functions.get_open_tabs", "<|constrain|>", "json", "<|message|>", `{"topic":"tabs"}`},
			reasoning: `I should check  to=functions.get_open_tabs<|constrain|>json<|message|>{"topic":"tabs"}`,
		},
		{
			name:    "channel recipient ended before ordinary analysis",
			tokens:  []string{"<|channel|>", "final", " to", "=", "functions", ".f", "<|channel|>", "analysis", "<|message|>", "reason", "<|end|>", "<|start|>", "assistant", "<|message|>", "answer"},
			content: "answer", reasoning: "reason",
		},
		{
			name:    "role recipient ended on second channel",
			tokens:  []string{"<|start|>", "assistant", " to", "=", "functions", ".f", "<|channel|>", "analysis", "<|channel|>", "final", "<|message|>", "answer", "<|end|>"},
			content: "answer",
		},
		{
			name:    "role recipient unknown first channel then supported ends candidate",
			tokens:  []string{"<|start|>", "assistant", " to", "=", "functions", ".f", "<|channel|>", "custom", "<|channel|>", "final", "<|message|>", "answer", "<|end|>"},
			content: "answer",
		},
		{
			name:   "fresh recipient excludes buffered old function",
			tokens: []string{"<|channel|>", "analysis", " to", "=", "functions", ".f", "<|channel|>", "comment", "ary", " to", "=", "functions", ".get", "_weather", "<|message|>", `{"city":"Paris"}`, "<|call|>"},
			calls:  []toolCallFn{{Name: "get_weather", Arguments: `{"city":"Paris"}`}},
		},
		{
			name:   "commentary tool path through call boundary",
			tokens: []string{"<|channel|>", "comment", "ary", " to", "=", "functions", ".f", "<|message|>", `{"x":1}`, "<|call|>"},
			calls:  []toolCallFn{{Name: "f", Arguments: `{"x":1}`}},
		},
		{
			name:   "analysis tool path through call boundary",
			tokens: []string{"<|channel|>", "analysis", " to", "=", "functions", ".f", "<|message|>", `{"x":1}`, "<|call|>"},
			calls:  []toolCallFn{{Name: "f", Arguments: `{"x":1}`}},
		},
		{
			name:   "analysis tool path through end boundary",
			tokens: []string{"<|channel|>", "analysis", " to", "=", "functions", ".f", "<|message|>", `{"x":1}`, "<|end|>"},
			calls:  []toolCallFn{{Name: "f", Arguments: `{"x":1}`}},
		},
		{
			name:    "analysis literal prefix preserved at end",
			tokens:  []string{"<|channel|>", "analysis", "<|message|>", "This is to=functions", "<|end|>", "<|start|>", "assistant", "<|channel|>", "final", "<|message|>", "answer"},
			content: "answer", reasoning: "This is to=functions",
		},
		{
			name:      "analysis literal prefix preserved at call",
			tokens:    []string{"<|channel|>", "analysis", "<|message|>", "This is to=functions", "<|call|>"},
			reasoning: "This is to=functions",
		},
		{
			name:   "unmatched commentary header tail is discarded",
			tokens: []string{"<|channel|>", "comment", "ary", " to", "=", "functions", "."},
		},
		{
			name:   "builtin body cannot become a generic function call",
			tokens: []string{"<|channel|>", "analysis", " to", "=", "python", "<|message|>", `{"name":"fake","arguments":{}}`, "<|call|>"},
		},
		{
			name:   "builtin body cannot contain a nested recipient",
			tokens: []string{"<|channel|>", "analysis", " to", "=", "python", "<|message|>", ` to=functions.fake<|message|>{}`, "<|end|>"},
		},
		{
			name:   "damaged JSON body is discarded at EOF",
			tokens: []string{"<|channel|>", "comment", "ary", " to", "=", "functions", ".f", "<|message|>", `{"x":`},
		},
		{
			name:    "complete tool body without end recovers before fresh final",
			tokens:  []string{"<|channel|>", "comment", "ary", " to", "=", "functions", ".f", "<|message|>", `{"x":1}`, "<|start|>", "assistant", "<|channel|>", "final", "<|message|>", "answer", "<|end|>"},
			content: "answer", calls: []toolCallFn{{Name: "f", Arguments: `{"x":1}`}},
		},
		{
			name:    "incomplete tool body discarded before fresh final",
			tokens:  []string{"<|channel|>", "comment", "ary", " to", "=", "functions", ".f", "<|message|>", `{"x":`, "<|start|>", "assistant", "<|channel|>", "final", "<|message|>", "answer", "<|end|>"},
			content: "answer",
		},
		{
			name:    "non JSON body ends before fresh final",
			tokens:  []string{"<|channel|>", "comment", "ary", " to", "=", "functions", ".f", "<|message|>", "not JSON", "<|end|>", "<|start|>", "assistant", "<|channel|>", "final", "<|message|>", "answer"},
			content: "answer",
		},
		{
			name:   "malformed arguments cannot recover a later object",
			tokens: []string{"<|channel|>", "analysis", " to", "=", "functions", ".f", "<|message|>", `oops {"x":1}`, "<|call|>"},
		},
		{
			name:   "Gemma4 empty thought followed by a call",
			tokens: []string{"<|channel>", "thought", "<channel|>", `<|tool_call>call:go{}<tool_call|>`},
			calls:  []toolCallFn{{Name: "go", Arguments: `{}`}},
		},
		{
			name:    "log headerless analysis then final",
			tokens:  []string{"<|channel|>", "analysis", "<|message|>", "Consider options.", "<|end|>", "<|start|>", "assistant", "<|channel|>", "final", "<|message|>", "Updated answer."},
			content: "Updated answer.", reasoning: "Consider options.",
		},
		{
			name:      "log analysis then constrained commentary call at EOF",
			tokens:    []string{"<|channel|>", "analysis", "<|message|>", "Consider options.", "<|end|>", "<|start|>", "assistant", "<|channel|>", "comment", "ary", " to", "=", "functions", ".f", " ", "<|constrain|>", "json", "<|message|>", `{"topic":""}`},
			reasoning: "Consider options.", calls: []toolCallFn{{Name: "f", Arguments: `{"topic":""}`}},
		},
		{
			name:    "log incomplete tool header ended before second thinking body",
			tokens:  []string{"<|channel|>", "analysis", "<|message|>", "Consider options.", "<|end|>", "<|start|>", "assistant", "<|channel|>", "comment", "ary", " to", "=", "functions", ".f", "<|channel|>", "comment", "ary", "<|channel|>", "analysis", "<|message|>", "Need an indoor option.", "<|end|>", "<|start|>", "assistant", "<|channel|>", "final", "<|message|>", "Updated answer."},
			content: "Updated answer.", reasoning: "Consider options.Need an indoor option.",
		},
	}
	for _, syntax := range []struct {
		name, text, args string
	}{
		{"bare JSON", `{"name":"f","arguments":{"x":1}}`, `{"x":1}`},
		{"Qwen wrapper", `<tool_call>{"name":"f","arguments":{"x":1}}</tool_call>`, `{"x":1}`},
		{"Qwen coder", `<tool_call><function=f><parameter=x>1</parameter></function></tool_call>`, `{"x":1}`},
		{"Gemma wrapper", `<|tool_call>call:f{x:1}<tool_call|>`, `{"x":1}`},
		{"MiniCPM XML", `<function name="f"><param name="x">1</param></function>`, `{"x":"1"}`},
		{"Ministral3 brackets", `[TOOL_CALLS]f[ARGS]{"x":1}`, `{"x":1}`},
		{"Spark tagged arguments", `<tool_call>f<arg_key>x</arg_key><arg_value>1</arg_value></tool_call>`, `{"x":"1"}`},
		{"LFM call", `<|tool_call_start|>[f(x=1)]<|tool_call_end|>`, `{"x":1}`},
		{"GPTOSS recipient text", ` to=functions.f<|message|>{"x":1}<|call|>`, `{"x":1}`},
	} {
		cases = append(cases,
			tokenCase{name: "literal reasoning/" + syntax.name, tokens: []string{"<think>", syntax.text, "</think>", "answer"}, content: "answer", reasoning: syntax.text},
			tokenCase{name: "content call/" + syntax.name, tokens: []string{"<think>", syntax.text, "</think>", syntax.text}, reasoning: syntax.text, calls: []toolCallFn{{Name: "f", Arguments: syntax.args}}},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser, splitter := NewToolCallParser(nil), thinkfsm.New()
			var output []ToolCallEvent
			for _, token := range tc.tokens {
				ev := thinkfsm.Event{Kind: thinkfsm.Content, Text: token}
				if !tc.plain {
					ev = splitter.Feed(token)
				}
				output = append(output, parser.Feed(ev)...)
			}
			output = append(output, parser.Finish()...)
			content, reasoning, calls := toolCallParserOutput(t, output)
			if content != tc.content || reasoning != tc.reasoning || !callsEqual(calls, tc.calls) {
				t.Errorf("output=(%q, %q, %+v), want (%q, %q, %+v)", content, reasoning, calls, tc.content, tc.reasoning, tc.calls)
			}
			if events := parser.Finish(); len(events) != 0 {
				t.Errorf("second Finish=%+v, want empty", events)
			}
		})
	}
}
