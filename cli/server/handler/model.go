// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package handler

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	geniex_sdk "github.com/qualcomm/GenieX/bindings/go"
)

// ModelResponse is the OpenAI model object plus GenieX's model_type extension.
type ModelResponse struct {
	openai.Model
	ModelType string `json:"model_type"`
}

func ListModels(c *gin.Context) {
	models, err := geniex_sdk.ModelListDetailed()
	if err != nil {
		c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	res := make([]openai.Model, 0, len(models))
	for _, m := range models {
		owner, _, _ := strings.Cut(m.Name, "/")
		for _, q := range m.Precisions {
			res = append(res, openai.Model{
				ID:      geniex_sdk.JoinNamePrecision(m.Name, q),
				OwnedBy: owner,
			})
		}
	}

	c.JSON(http.StatusOK, map[string]any{
		"object": "list",
		"data":   res,
	})
}

func RetrieveModel(c *gin.Context) {
	name, quant := geniex_sdk.SplitNamePrecision(strings.TrimPrefix(c.Param("model"), "/"))

	// ModelGetDetailed canonicalizes the name in the SDK, so an alias form the
	// store never records under (a bare AI Hub id) still resolves.
	m, err := geniex_sdk.ModelGetDetailed(name)
	if err != nil {
		if geniex_sdk.IsModelNotFound(err) {
			c.JSON(http.StatusNotFound, nil)
		} else {
			c.JSON(http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
		return
	}

	if quant == "" {
		// The head is a bare name's own pick; sorting would advertise another.
		if len(m.Precisions) == 0 {
			c.JSON(http.StatusNotFound, nil)
			return
		}
		quant = m.Precisions[0]
	} else if i := slices.IndexFunc(m.Precisions, func(p string) bool {
		return strings.EqualFold(p, quant)
	}); i < 0 {
		c.JSON(http.StatusNotFound, nil)
		return
	} else {
		// echo the precision as the store spells it, not as the caller cased it
		quant = m.Precisions[i]
	}

	owner, _, _ := strings.Cut(m.Name, "/")
	c.JSON(http.StatusOK, ModelResponse{
		Model: openai.Model{
			ID:      geniex_sdk.JoinNamePrecision(m.Name, quant),
			OwnedBy: owner,
		},
		ModelType: m.ModelType.String(),
	})
}
