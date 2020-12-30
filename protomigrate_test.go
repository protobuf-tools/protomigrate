// Copyright 2020 The protobuf-tools Authors.
// SPDX-License-Identifier: BSD-3-Clause

package protomigrate_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/protobuf-tools/protomigrate"
)

// TestAnalyzer is a test for Analyzer.
func TestAnalyzer(t *testing.T) {
	t.Parallel()

	testdata := analysistest.TestData()

	tests := map[string]struct {
		name string
	}{
		"a": {
			name: "a",
		},
		"CheckDeprecated": {
			name: "check_deprecated",
		},
	}
	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.Command("go", "mod", "vendor")
			cmd.Dir = filepath.Join(testdata, "src", tt.name)
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				os.RemoveAll(filepath.Join(testdata, "src", tt.name, "vendor"))
			})

			analysistest.Run(t, testdata, protomigrate.Analyzer, tt.name)
		})
	}
}
