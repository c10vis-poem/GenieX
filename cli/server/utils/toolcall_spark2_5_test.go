// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package utils

import "testing"

func TestSpark25ToolCallScanner(t *testing.T) {
	const tools = `[
		{"type":"function","function":{"name":"f","parameters":{"type":"object","properties":{
			"count":{"type":"integer"},"price":{"type":"number"},"enabled":{"type":"boolean"},
			"config":{"type":"object"},"items":{"type":"array"},"nothing":{"type":"null"},
			"label":{"type":"string"},"unsupported":{"type":["integer","string"]}
		}}}},
		{"type":"function","function":{"name":"other","parameters":{"type":"object","properties":{
			"count":{"type":"string"}
		}}}}
	]`
	types := ToolParameterTypesFromTools(tools)
	type scannerCase struct {
		name      string
		resp      string
		wantText  string
		wantCalls []toolCallFn
	}
	tests := []scannerCase{
		{
			name:      "string parameter",
			resp:      `<tool_call>get_weather<arg_key>city</arg_key><arg_value>Beijing</arg_value></tool_call>`,
			wantCalls: []toolCallFn{{Name: "get_weather", Arguments: `{"city":"Beijing"}`}},
		},
		{
			name:      "empty arguments",
			resp:      `<tool_call>get_time</tool_call>`,
			wantCalls: []toolCallFn{{Name: "get_time", Arguments: `{}`}},
		},
		{
			name:      "empty value",
			resp:      `<tool_call>f<arg_key>label</arg_key><arg_value></arg_value></tool_call>`,
			wantCalls: []toolCallFn{{Name: "f", Arguments: `{"label":""}`}},
		},
		{
			name: "typed values and per-function schema",
			resp: `<tool_call>f` +
				`<arg_key>count</arg_key><arg_value>1e0</arg_value>` +
				`<arg_key>price</arg_key><arg_value>2.5</arg_value>` +
				`<arg_key>enabled</arg_key><arg_value>true</arg_value>` +
				`<arg_key>config</arg_key><arg_value>{"options":{"strict":true}}</arg_value>` +
				`<arg_key>items</arg_key><arg_value>["alpha",{"id":1}]</arg_value>` +
				`<arg_key>nothing</arg_key><arg_value>null</arg_value>` +
				`<arg_key>label</arg_key><arg_value>5</arg_value>` +
				`</tool_call><tool_call>other<arg_key>count</arg_key><arg_value>5</arg_value></tool_call>`,
			wantCalls: []toolCallFn{
				{Name: "f", Arguments: `{"count":1e0,"price":2.5,"enabled":true,"config":{"options":{"strict":true}},"items":["alpha",{"id":1}],"nothing":null,"label":"5"}`},
				{Name: "other", Arguments: `{"count":"5"}`},
			},
		},
		{
			name: "untyped and unsupported schemas preserve strings",
			resp: `<tool_call>f<arg_key>unknown</arg_key><arg_value>5</arg_value>` +
				`<arg_key>unsupported</arg_key><arg_value>true</arg_value></tool_call>`,
			wantCalls: []toolCallFn{{Name: "f", Arguments: `{"unknown":"5","unsupported":"true"}`}},
		},
		{
			name: "invalid typed values preserve strings",
			resp: `<tool_call>f` +
				`<arg_key>count</arg_key><arg_value>1.5</arg_value>` +
				`<arg_key>price</arg_key><arg_value>abc</arg_value>` +
				`<arg_key>enabled</arg_key><arg_value>yes</arg_value>` +
				`<arg_key>config</arg_key><arg_value>[]</arg_value>` +
				`<arg_key>items</arg_key><arg_value>{}</arg_value>` +
				`<arg_key>nothing</arg_key><arg_value>false</arg_value></tool_call>`,
			wantCalls: []toolCallFn{{Name: "f", Arguments: `{"count":"1.5","price":"abc","enabled":"yes","config":"[]","items":"{}","nothing":"false"}`}},
		},
		{
			name:      "whitespace between tags and raw multiline unicode value",
			resp:      "<tool_call>\nwrite\n<arg_key>body</arg_key>\n<arg_value>\n  上海 & <tag>\n\"quoted\"\n</arg_value>\n</tool_call>",
			wantCalls: []toolCallFn{{Name: "write", Arguments: `{"body":"\n  上海 & <tag>\n\"quoted\"\n"}`}},
		},
		{
			name:      "tags in a value are raw text",
			resp:      `<tool_call>f<arg_key>label</arg_key><arg_value><arg_key>x</arg_key> <tool_call> &amp;</arg_value></tool_call>`,
			wantCalls: []toolCallFn{{Name: "f", Arguments: `{"label":"<arg_key>x</arg_key> <tool_call> &amp;"}`}},
		},
		{
			name:     "arg value close tag in raw value falls back to text",
			resp:     `<tool_call>f<arg_key>label</arg_key><arg_value>before </arg_value> after</arg_value></tool_call>`,
			wantText: `<tool_call>f<arg_key>label</arg_key><arg_value>before </arg_value> after</arg_value></tool_call>`,
		},
		{
			name:     "call close tag in raw value falls back to text",
			resp:     `<tool_call>write<arg_key>content</arg_key><arg_value>close with </tool_call> tag</arg_value></tool_call>`,
			wantText: `<tool_call>write<arg_key>content</arg_key><arg_value>close with </tool_call> tag</arg_value></tool_call>`,
		},
		{
			name:      "JSON-looking string does not become a nested call",
			resp:      `<tool_call>f<arg_key>label</arg_key><arg_value>{"name":"nested","arguments":{}}</arg_value></tool_call>`,
			wantCalls: []toolCallFn{{Name: "f", Arguments: `{"label":"{\"name\":\"nested\",\"arguments\":{}}"}`}},
		},
		{
			name: "parallel calls preserve prose",
			resp: `Checking. <tool_call>f<arg_key>count</arg_key><arg_value>5</arg_value></tool_call>` +
				` then <tool_call>get_time</tool_call> Done.`,
			wantText:  "Checking.  then  Done.",
			wantCalls: []toolCallFn{{Name: "f", Arguments: `{"count":5}`}, {Name: "get_time", Arguments: `{}`}},
		},
		{
			name: "shared wrapper still accepts JSON and Qwen coder",
			resp: `<tool_call>{"name":"json","arguments":{"x":1}}</tool_call>` +
				`<tool_call><function=coder><parameter=x>1</parameter></function></tool_call>` +
				`<tool_call>get_time</tool_call>`,
			wantCalls: []toolCallFn{{Name: "json", Arguments: `{"x":1}`}, {Name: "coder", Arguments: `{"x":1}`}, {Name: "get_time", Arguments: `{}`}},
		},
	}
	for _, resp := range []string{
		`<tool_call>not a call at all</tool_call>`,
		`<tool_call></tool_call>`,
		`<tool_call><arg_key>x</arg_key><arg_value>1</arg_value></tool_call>`,
		`<tool_call>f<arg_key></arg_key><arg_value>1</arg_value></tool_call>`,
		`<tool_call>f<arg_key>x</arg_key><arg_value>1</arg_value><arg_key>x</arg_key><arg_value>2</arg_value></tool_call>`,
		`<tool_call>f<arg_key>x<arg_value>1</arg_value></tool_call>`,
		`<tool_call>f<arg_key>x</arg_key>1</tool_call>`,
		`<tool_call>f<arg_key>x</arg_key><arg_value>1</tool_call>`,
		`<tool_call>f<arg_key>x</arg_key><arg_value>1</arg_value>garbage</tool_call>`,
		`<tool_call>f<arg_key>x</arg_key><arg_value>1</arg_value>`,
		`<tool_call>get_time`,
		`<tool_call>`,
		`<tool_c`,
	} {
		tests = append(tests, scannerCase{name: "malformed/" + resp, resp: resp, wantText: resp})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkToolCallScanner(t, tt.resp, tt.wantText, tt.wantCalls, types)
		})
	}
}
