// Copyright 2024-2026 Qualcomm Technologies, Inc. and/or its subsidiaries.
// SPDX-License-Identifier: BSD-3-Clause

package render

import (
	"fmt"
	"os"
	"sync"

	"github.com/charmbracelet/huh/spinner"
)

type Spinner struct {
	*spinner.Spinner
	start sync.WaitGroup
	stop  sync.WaitGroup
}

func NewSpinner(desc string) *Spinner {
	s := Spinner{
		Spinner: spinner.New().Title(desc).Type(spinner.Globe),
	}

	s.Action(func() { s.start.Wait() })

	return &s
}

func (s *Spinner) Start() {
	s.start.Add(1)
	s.stop.Go(func() {
		// if NO_COLOR is set, do not show spinner
		if os.Getenv("NO_COLOR") == "1" {
			fmt.Println(s.View())
			s.start.Wait()
		} else {
			s.Spinner.Run()
		}
	})
}

func (s *Spinner) Stop() {
	s.start.Done()
	s.stop.Wait()
}
