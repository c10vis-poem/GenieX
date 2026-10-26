// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package utils

import (
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"github.com/bytedance/sonic"
)

// MiniCPM5 emits one XML element for each call, without an outer wrapper:
//
//	<function name="get_weather"><param name="city">Beijing</param></function>
//
// Parameter values can be multiline and, for special characters, wrapped in a
// CDATA block. encoding/xml handles both forms and decodes XML entities before
// the values are re-encoded as the JSON object OpenAI-compatible clients expect.
const (
	miniCPM5FunctionOpen  = "<function name="
	miniCPM5FunctionClose = "</function>"
)

type miniCPM5ToolCall struct {
	begin     markerScan
	closeFrom int // first byte where an untried possible end may begin
	types     map[string]map[string]string
}

func newMiniCPM5ToolCall(types map[string]map[string]string) *miniCPM5ToolCall {
	return &miniCPM5ToolCall{begin: markerScan{marker: miniCPM5FunctionOpen}, types: types}
}

func (t *miniCPM5ToolCall) parse(s string) []toolCallFn { return parseMiniCPM5ToolCalls(s, t.types) }

func (t *miniCPM5ToolCall) feed(all string, from int) (int, int) {
	if from > t.begin.start {
		t.begin.reset(from)
		t.closeFrom = from
	}
	t.begin.feed(all)
	if t.begin.done == 0 {
		if t.begin.start < len(all) {
			return t.begin.start, -1
		}
		return -1, -1
	}
	at := t.begin.done - len(miniCPM5FunctionOpen)
	t.closeFrom = max(t.closeFrom, t.begin.done)
	for {
		closeAt := strings.Index(all[t.closeFrom:], miniCPM5FunctionClose)
		selfAt := strings.Index(all[t.closeFrom:], "/>")
		if closeAt < 0 && selfAt < 0 {
			// Keep only the suffix that could start an end marker across chunks.
			t.closeFrom = max(t.closeFrom, len(all)-len(miniCPM5FunctionClose)+1)
			return at, -1
		}
		marker := miniCPM5FunctionClose
		if selfAt >= 0 && (closeAt < 0 || selfAt < closeAt) {
			closeAt, marker = selfAt, "/>"
		}
		t.closeFrom += closeAt + len(marker)
		_, end, err := decodeMiniCPM5Function(all[at:t.closeFrom])
		if err == nil {
			return at, at + end
		}
		var syntax *xml.SyntaxError
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
			(errors.As(err, &syntax) && strings.HasPrefix(syntax.Msg, "unexpected EOF")) {
			// A closer inside CDATA or a self-closed param may not end the function.
			continue
		}
		// A syntax error cannot become valid by appending tokens. Give the
		// rejected prefix back as text so later calls do not wait for Tail.
		return at, at + max(end, len(miniCPM5FunctionOpen))
	}
}

type miniCPM5Function struct {
	XMLName xml.Name        `xml:"function"`
	Name    string          `xml:"name,attr"`
	Params  []miniCPM5Param `xml:"param"`
}

type miniCPM5Param struct {
	Name  string `xml:"name,attr"`
	Value string `xml:",chardata"`
}

func decodeMiniCPM5Function(s string) (miniCPM5Function, int, error) {
	var fn miniCPM5Function
	decoder := xml.NewDecoder(strings.NewReader(s))
	err := decoder.Decode(&fn)
	return fn, int(decoder.InputOffset()), err
}

// parseMiniCPM5ToolCalls returns every complete, well-formed MiniCPM5 function
// element in s. Invalid XML is deliberately not repaired: treating malformed
// model text as an executable call would be unsafe.
func parseMiniCPM5ToolCalls(s string, types map[string]map[string]string) []toolCallFn {
	var calls []toolCallFn
	for {
		i := strings.Index(s, miniCPM5FunctionOpen)
		if i < 0 {
			return calls
		}
		s = s[i:]

		fn, end, err := decodeMiniCPM5Function(s)
		if err != nil {
			s = s[len(miniCPM5FunctionOpen):]
			continue
		}
		s = s[end:]
		if fn.Name == "" {
			continue
		}

		args := miniCPM5Arguments(fn.Params, types[fn.Name])
		calls = append(calls, toolCallFn{Name: fn.Name, Arguments: args})
	}
}

func miniCPM5Arguments(params []miniCPM5Param, types map[string]string) string {
	var b strings.Builder
	b.WriteByte('{')
	for _, param := range params {
		if param.Name == "" {
			continue
		}
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		name, _ := sonic.MarshalString(param.Name)
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(toolArgumentValue(param.Value, types[param.Name]))
	}
	b.WriteByte('}')
	return b.String()
}
