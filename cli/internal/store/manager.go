// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package store

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/qualcomm/GenieX/cli/internal/config"
	"github.com/qualcomm/GenieX/cli/internal/render"
)

// Store resolves the geniex data directory for the CLI. Model storage itself
// is owned by the SDK model-manager; this only exposes the data-dir root (for
// the config file, update cache, and REPL history) via DataPath().
type Store struct {
	home string
}

var instance = sync.OnceValue(func() *Store {
	s := &Store{}
	s.init()
	return s
})

// Get returns the singleton instance of Store.
func Get() *Store {
	return instance()
}

// init resolves the data directory. The SDK model-manager creates the models
// subdirectory itself, so the store only needs the root to exist.
func (s *Store) init() {
	if config.Get().DataDir != "" {
		s.home = config.Get().DataDir
	} else {
		homeDir, e := os.UserHomeDir()
		if e != nil {
			fatal("resolve user home directory", e)
		}
		s.home = filepath.Join(homeDir, ".cache", "geniex")
	}
	slog.Info("Using data directory", "path", s.home)

	if e := os.MkdirAll(s.home, 0o770); e != nil {
		fatal(fmt.Sprintf("create data directory %s", s.home), e)
	}
}

// fatal reports an unusable data directory and exits; Get() is a sync.OnceValue with
// no error channel. Prints as well as logs: the default log level is "none".
func fatal(what string, err error) {
	slog.Error(what, "err", err)
	theme := render.GetTheme()
	fmt.Fprintln(os.Stderr, theme.Error.Sprintf("Error: %s: %s", what, err))
	fmt.Fprintln(os.Stderr, theme.Error.Sprint("Point --data-dir (or GENIEX_DATADIR) at a writable directory."))
	os.Exit(1)
}

// DataPath returns the geniex data directory root.
func (s *Store) DataPath() string {
	return s.home
}
