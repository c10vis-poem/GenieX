// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package utils

import (
	"encoding/json"
	"strings"
)

const (
	ministral3Open = "[TOOL_CALLS]"
	ministral3Args = "[ARGS]"
)

// Ministral3 calls end at the closing JSON brace, without an end marker.
type ministral3ToolCall struct {
	begin                   markerScan
	pos, nameEnd, body, end int
	walk                    braceWalk
}

func newMinistral3ToolCall() *ministral3ToolCall {
	return &ministral3ToolCall{begin: markerScan{marker: ministral3Open}}
}

func (t *ministral3ToolCall) feed(all string, from int) (int, int) {
	if from > t.begin.start {
		t.begin.reset(from)
		t.pos, t.nameEnd, t.body, t.end = from, 0, 0, 0
		t.walk = braceWalk{}
	}
	t.begin.feed(all)
	if t.begin.done == 0 {
		if t.begin.start < len(all) {
			return t.begin.start, -1
		}
		return -1, -1
	}
	at := t.begin.start
	if t.end > 0 {
		return at, t.end
	}
	t.pos = max(t.pos, t.begin.done)
	for t.nameEnd == 0 && t.pos < len(all) {
		switch all[t.pos] {
		case '[':
			marker := all[t.pos:min(t.pos+len(ministral3Args), len(all))]
			if t.pos == t.begin.done || !strings.HasPrefix(ministral3Args, marker) {
				t.end = t.pos
				return at, t.end
			}
			if len(marker) < len(ministral3Args) {
				return at, -1
			}
			t.nameEnd = t.pos
			t.pos += len(ministral3Args)
		case ']', '{', '}', ' ', '\t', '\r', '\n':
			t.end = t.pos
			return at, t.end
		default:
			t.pos++
		}
	}
	if t.nameEnd == 0 {
		return at, -1
	}
	if t.body == 0 {
		t.pos = skipSpace(all, t.pos)
		if t.pos == len(all) {
			return at, -1
		}
		if all[t.pos] != '{' {
			t.end = t.pos
			return at, t.end
		}
		t.body = t.pos
	}
	n := t.walk.feed(all[t.pos:])
	if n < 0 {
		t.pos = len(all)
		return at, -1
	}
	t.pos += n
	t.end = t.pos
	return at, t.end
}

func (t *ministral3ToolCall) parse(s string) []toolCallFn {
	scan := newMinistral3ToolCall()
	var calls []toolCallFn
	inString, escaped := false, false
	for from := 0; from < len(s); {
		if inString {
			switch {
			case escaped:
				escaped = false
			case s[from] == '\\':
				escaped = true
			case s[from] == '"':
				inString = false
			}
			from++
			continue
		}
		if s[from] == '"' {
			inString = true
			from++
			continue
		}
		if !strings.HasPrefix(s[from:], ministral3Open) {
			from++
			continue
		}
		_, end := scan.feed(s, from)
		if end < 0 {
			break
		}
		// sonic.ValidString can accept invalid JSON on arm64 when the string ends mid-buffer.
		if scan.body > 0 && json.Valid([]byte(s[scan.body:end])) {
			calls = append(calls, toolCallFn{
				Name: s[scan.begin.done:scan.nameEnd], Arguments: s[scan.body:end],
			})
		}
		from = end
	}
	return calls
}
