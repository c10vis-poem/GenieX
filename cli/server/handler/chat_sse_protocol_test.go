// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package handler

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	geniex_sdk "github.com/qualcomm/GenieX/bindings/go"
)

func TestChatSSEProtocol(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tool   bool
		tokens []string
		err    error
	}{
		{name: "plain buffered thinking", tokens: []string{"<think>", "reason", "</think>", "answer"}},
		{name: "tool buffered wrapper", tool: true, tokens: []string{"<tool_call>", `{"name":"f","arguments":{}}`, "</tool_call>"}},
		{name: "tool EOF recovery", tool: true, tokens: []string{"<tool_call>", `{"name":"f","arguments":{}}`}},
		{name: "empty stream"},
		{name: "generation error", tool: true, err: errors.New("generation failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := &streamRecorder{httptest.NewRecorder(), make(chan bool)}
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			dataCh := make(chan string, len(tc.tokens))
			for _, token := range tc.tokens {
				dataCh <- token
			}
			close(dataCh)
			profile := geniex_sdk.ProfileData{StopReason: "eos"}
			before := time.Now().Unix()
			wait := func() error { return tc.err }
			if tc.tool {
				streamToolCall(c, dataCh, wait, true, &profile, reasoningClass(), "", "test-model")
			} else {
				streamPlainText(c, dataCh, wait, true, &profile, reasoningClass(), "test-model")
			}
			contentType := w.Result().Header.Get("Content-Type")
			if got, _, err := mime.ParseMediaType(contentType); err != nil || got != "text/event-stream" {
				t.Errorf("committed Content-Type = %q, want text/event-stream", contentType)
			}
			if got := w.Result().Header.Get("Cache-Control"); got != "no-cache" {
				t.Errorf("committed Cache-Control = %q, want no-cache", got)
			}
			var id string
			var created int64
			chunks, usage, done, sawError := 0, 0, false, false
			for line := range strings.SplitSeq(w.Body.String(), "\n") {
				data, ok := strings.CutPrefix(line, "data:")
				if !ok {
					continue
				}
				if strings.TrimSpace(data) == "[DONE]" {
					done = true
					continue
				}
				var chunk struct {
					ID      string                  `json:"id"`
					Created int64                   `json:"created"`
					Model   string                  `json:"model"`
					Object  string                  `json:"object"`
					Usage   *openai.CompletionUsage `json:"usage"`
					Error   string                  `json:"error"`
				}
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					t.Fatal(err)
				}
				if chunk.Error != "" {
					if tc.err == nil || chunk.Error != tc.err.Error() {
						t.Fatalf("unexpected error event: %s", data)
					}
					sawError = true
					continue
				}
				if !strings.HasPrefix(chunk.ID, "chatcmpl-") || chunk.Model != "test-model" || chunk.Created < before || chunk.Created > time.Now().Unix() {
					t.Fatalf("incomplete stream envelope: %s", data)
				}
				if chunks > 0 && (chunk.ID != id || chunk.Created != created) {
					t.Fatalf("stream metadata changed: %s", data)
				}
				if chunk.Object != streamChunkObject {
					t.Fatalf("object = %q", chunk.Object)
				}
				id, created = chunk.ID, chunk.Created
				chunks++
				if chunk.Usage != nil {
					usage++
				}
			}
			if tc.err == nil && (chunks < 2 || usage != 1 || !done) {
				t.Fatalf("chunks=%d usage=%d done=%v", chunks, usage, done)
			}
			if tc.err != nil && (!sawError || chunks != 0 || usage != 0 || done) {
				t.Fatalf("error=%v chunks=%d usage=%d done=%v", sawError, chunks, usage, done)
			}
		})
	}
}
