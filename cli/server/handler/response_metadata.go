// Copyright (c) 2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package handler

import (
	"fmt"
	"math/rand/v2"
	"time"
)

type responseMetadata struct {
	ID      string `json:"id"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
}

func newResponseMetadata(prefix, model string) responseMetadata {
	return responseMetadata{
		ID:      fmt.Sprintf("%s-%016x", prefix, rand.Uint64()),
		Created: time.Now().Unix(),
		Model:   model,
	}
}
