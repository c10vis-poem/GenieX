// Copyright (c) 2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
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

	geniex_sdk "github.com/qualcomm/GenieX/bindings/go"
)

func checkResponseMetadata(t *testing.T, body []byte, prefix, model string) responseMetadata {
	t.Helper()
	var meta responseMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if !strings.HasPrefix(meta.ID, prefix+"-") || meta.Model != model || meta.Created <= 0 || meta.Created > now || now-meta.Created > 5 {
		t.Fatalf("incomplete response metadata: %s", body)
	}
	return meta
}

func TestCompletionResponseProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	seen := map[string]bool{}
	for _, text := range []string{"hello", ""} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		profile := geniex_sdk.ProfileData{PromptTokens: 3, GeneratedTokens: 2, StopReason: "length"}
		writeCompletionResponse(c, text, profile, "test-model")
		meta := checkResponseMetadata(t, w.Body.Bytes(), "cmpl", "test-model")
		if seen[meta.ID] {
			t.Fatalf("reused response ID %q", meta.ID)
		}
		seen[meta.ID] = true
		var got completionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || got.Object != "text_completion" || len(got.Choices) != 1 || got.Choices[0].Text != text || got.Choices[0].FinishReason != "length" || got.Usage.TotalTokens != 5 {
			t.Fatalf("response: %s", w.Body.Bytes())
		}
	}
}

func TestCompletionSSEProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	seen := map[string]bool{}
	for _, tc := range []struct {
		name   string
		tokens []string
		echo   string
		usage  bool
		err    error
	}{
		{name: "text and usage", tokens: []string{"hello", " world"}, usage: true},
		{name: "echo", tokens: []string{" world"}, echo: "hello"},
		{name: "empty", usage: true},
		{name: "context limit", tokens: []string{"partial"}, err: geniex_sdk.ErrLlmTokenizationContextLength, usage: true},
		{name: "error", err: errors.New("generation failed"), usage: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &streamRecorder{httptest.NewRecorder(), make(chan bool)}
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/completions", nil)
			ch := make(chan string, len(tc.tokens))
			for _, token := range tc.tokens {
				ch <- token
			}
			close(ch)
			profile := geniex_sdk.ProfileData{PromptTokens: 3, GeneratedTokens: 2, StopReason: "eos"}
			if tc.err == geniex_sdk.ErrLlmTokenizationContextLength {
				profile.StopReason = "length"
			}
			streamCompletion(c, ch, func() error { return tc.err }, tc.usage, tc.echo, &profile, "test-model")
			result := w.Result()
			mediaType, _, mediaTypeErr := mime.ParseMediaType(result.Header.Get("Content-Type"))
			if mediaTypeErr != nil || mediaType != "text/event-stream" || result.Header.Get("Cache-Control") != "no-cache" {
				t.Fatalf("committed headers: %v", result.Header)
			}
			if result.StatusCode != 200 {
				t.Fatalf("committed status: %d", result.StatusCode)
			}
			var meta responseMetadata
			var text strings.Builder
			chunks, textChunks, finishes, usage, done, failures := 0, 0, 0, 0, 0, 0
			for _, line := range strings.Split(w.Body.String(), "\n") {
				if !strings.HasPrefix(line, "data:") {
					continue
				}
				body := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if body == "[DONE]" {
					done++
					continue
				}
				var errorBody struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal([]byte(body), &errorBody); err != nil {
					t.Fatal(err)
				}
				if errorBody.Error != "" {
					failures++
					continue
				}
				gotMeta := checkResponseMetadata(t, []byte(body), "cmpl", "test-model")
				if chunks == 0 {
					if seen[gotMeta.ID] {
						t.Fatalf("reused response ID %q", gotMeta.ID)
					}
					seen[gotMeta.ID] = true
					meta = gotMeta
				} else if gotMeta != meta {
					t.Fatalf("unstable metadata: %s", body)
				}
				chunks++
				var got completionStreamChunk
				if err := json.Unmarshal([]byte(body), &got); err != nil {
					t.Fatal(err)
				}
				if got.Object != "text_completion" {
					t.Fatalf("object: %s", body)
				}
				if got.Usage != nil {
					usage++
					if len(got.Choices) != 0 || got.Usage.TotalTokens != 5 {
						t.Fatalf("usage: %s", body)
					}
				}
				for _, choice := range got.Choices {
					text.WriteString(choice.Text)
					if choice.FinishReason != nil {
						finishes++
						if *choice.FinishReason != mapFinishReason(profile.StopReason) {
							t.Fatalf("finish: %s", body)
						}
					} else {
						textChunks++
					}
				}
			}
			if text.String() != tc.echo+strings.Join(tc.tokens, "") {
				t.Fatalf("text=%q", text.String())
			}
			if tc.err != nil && tc.err != geniex_sdk.ErrLlmTokenizationContextLength {
				if failures != 1 || done != 0 || usage != 0 || finishes != 0 {
					t.Fatalf("error stream: %s", w.Body.String())
				}
			} else {
				wantUsage := 0
				if tc.usage {
					wantUsage = 1
				}
				wantTextChunks := len(tc.tokens)
				if tc.echo != "" {
					wantTextChunks++
				}
				if failures != 0 || done != 1 || finishes != 1 || usage != wantUsage || textChunks != wantTextChunks {
					t.Fatalf("terminal stream: text=%d finish=%d usage=%d done=%d body=%s", textChunks, finishes, usage, done, w.Body.String())
				}
			}
		})
	}
}
