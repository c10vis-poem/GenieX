// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package types

import (
	"fmt"
	"math/rand/v2"
	"time"
)

// ResponseMetadata holds the OpenAI envelope fields shared by response bodies.
type ResponseMetadata struct {
	ID      string `json:"id"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
}

// NewResponseMetadata stamps a fresh ID and creation time for one response.
func NewResponseMetadata(prefix, model string) ResponseMetadata {
	return ResponseMetadata{
		ID:      fmt.Sprintf("%s-%016x", prefix, rand.Uint64()),
		Created: time.Now().Unix(),
		Model:   model,
	}
}
