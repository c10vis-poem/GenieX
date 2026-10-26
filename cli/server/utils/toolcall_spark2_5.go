// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package utils

import (
	"strings"

	"github.com/bytedance/sonic"
)

const (
	spark25CallOpen   = "<tool_call>"
	spark25CallClose  = "</tool_call>"
	spark25KeyOpen    = "<arg_key>"
	spark25KeyClose   = "</arg_key>"
	spark25ValueOpen  = "<arg_value>"
	spark25ValueClose = "</arg_value>"
)

type spark25ToolCall struct {
	markerFormat
	types map[string]map[string]string
}

func newSpark25ToolCall(types map[string]map[string]string) *spark25ToolCall {
	return &spark25ToolCall{markerFormat: newMarkerFormat(spark25CallOpen, spark25CallClose), types: types}
}

func (t *spark25ToolCall) parse(s string) []toolCallFn {
	var calls []toolCallFn
	for {
		start := strings.Index(s, spark25CallOpen)
		if start < 0 {
			return calls
		}
		s = s[start+len(spark25CallOpen):]
		end := strings.Index(s, spark25CallClose)
		if end < 0 {
			return calls
		}
		body := s[:end]
		s = s[end+len(spark25CallClose):]
		if call := parseSpark25Call(body, t.types); call != nil {
			calls = append(calls, *call)
		}
	}
}

func parseSpark25Call(body string, types map[string]map[string]string) *toolCallFn {
	end := strings.IndexByte(body, '<')
	if end < 0 {
		end = len(body)
	}
	name := strings.TrimSpace(body[:end])
	if name == "" {
		return nil
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return nil
		}
	}
	body = body[end:]
	var args strings.Builder
	args.WriteByte('{')
	seen := make(map[string]bool)
	for body = strings.TrimSpace(body); body != ""; body = strings.TrimSpace(body) {
		var ok bool
		body, ok = strings.CutPrefix(body, spark25KeyOpen)
		if !ok {
			return nil
		}
		key, rest, ok := strings.Cut(body, spark25KeyClose)
		if !ok || key == "" || seen[key] {
			return nil
		}
		body, ok = strings.CutPrefix(strings.TrimSpace(rest), spark25ValueOpen)
		if !ok {
			return nil
		}
		value, rest, ok := strings.Cut(body, spark25ValueClose)
		if !ok {
			return nil
		}
		body = rest
		seen[key] = true
		if args.Len() > 1 {
			args.WriteByte(',')
		}
		quoted, _ := sonic.MarshalString(key)
		args.WriteString(quoted)
		args.WriteByte(':')
		args.WriteString(toolArgumentValue(value, types[name][key]))
	}
	args.WriteByte('}')
	return &toolCallFn{Name: name, Arguments: args.String()}
}
