// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package utils

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"

	_ "golang.org/x/image/webp"
)

func SaveURIToTempFile(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	var data []byte
	// windows drive letter
	if len(u.Scheme) == 1 && (u.Scheme[0] >= 'a' && u.Scheme[0] <= 'z' || u.Scheme[0] >= 'A' && u.Scheme[0] <= 'Z') {
		u.Scheme = "file"
		u.Path = uri
	}
	switch u.Scheme {
	case "http", "https":
		resp, err := http.Get(uri)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", errors.New("http download failed: " + resp.Status)
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
	case "data":
		header, payload, ok := strings.Cut(u.Opaque, ",")
		if !ok {
			return "", errors.New("invalid data URI format")
		}
		// format: data:[<mediatype>][;base64],<data>
		if strings.Contains(header, ";base64") {
			data, err = base64.StdEncoding.DecodeString(payload)
			if err != nil {
				return "", err
			}

		} else {
			// format: data:[<mediatype>],<data>
			decoded, err := url.QueryUnescape(payload)
			if err != nil {
				return "", err
			}
			data = []byte(decoded)
		}
	case "file", "":
		data, err = os.ReadFile(u.Path)
		if err != nil {
			return "", err
		}
	default:

		return "", errors.New("unsupported scheme: " + u.Scheme)
	}

	// Detect content type
	contentType := http.DetectContentType(data)

	// Convert WebP to PNG for compatibility with native SDK
	if strings.HasPrefix(contentType, "image/webp") || strings.HasSuffix(strings.ToLower(u.Path), ".webp") {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", errors.New("failed to decode WebP image: " + err.Error())
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return "", errors.New("failed to encode image as PNG: " + err.Error())
		}

		data = buf.Bytes()
		contentType = "image/png"
	}

	fileExt := ""
	if exts, err := mime.ExtensionsByType(contentType); err == nil && len(exts) > 0 {
		fileExt = exts[0]
	}

	tmpFile, err := os.CreateTemp("", "uri-*"+fileExt)
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	_, err = tmpFile.Write(data)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	return tmpFile.Name(), nil
}
