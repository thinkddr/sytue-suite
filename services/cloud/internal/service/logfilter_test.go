// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"strings"
	"testing"
)

const sampleLog = `INFO[0001] starting build
DEBUG[0002] resolving deps
error: package not found
WARN[0003] retrying
INFO[0004] build complete
`

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n")
}

func TestFilterLogLines(t *testing.T) {
	t.Run("sin filtro devuelve todo", func(t *testing.T) {
		out := filterLogLines(strings.NewReader(sampleLog), "", "", 0)
		if lineCount(out) != 5 {
			t.Errorf("esperaba 5 líneas; got %d: %q", lineCount(out), out)
		}
	})

	t.Run("substring case-insensitive", func(t *testing.T) {
		out := filterLogLines(strings.NewReader(sampleLog), "BUILD", "", 0)
		if lineCount(out) != 2 || !strings.Contains(out, "starting build") || !strings.Contains(out, "build complete") {
			t.Errorf("q=BUILD debe casar 2 líneas con 'build'; got %q", out)
		}
	})

	t.Run("level casa como token", func(t *testing.T) {
		out := filterLogLines(strings.NewReader(sampleLog), "", "info", 0)
		if lineCount(out) != 2 {
			t.Errorf("level=info debe casar 2 líneas INFO; got %q", out)
		}
	})

	t.Run("level no casa subcadena de otra palabra", func(t *testing.T) {
		out := filterLogLines(strings.NewReader("INFORMATION only\nINFO real\n"), "", "INFO", 0)
		if lineCount(out) != 1 || !strings.Contains(out, "INFO real") {
			t.Errorf("level=INFO no debe casar INFORMATION; got %q", out)
		}
	})

	t.Run("q y level combinan en AND", func(t *testing.T) {
		out := filterLogLines(strings.NewReader(sampleLog), "build", "info", 0)
		if lineCount(out) != 2 { // INFO ∧ 'build': las dos líneas INFO llevan 'build'
			t.Errorf("q=build ∧ level=info debe casar 2 líneas; got %q", out)
		}
		out2 := filterLogLines(strings.NewReader(sampleLog), "retrying", "info", 0)
		if lineCount(out2) != 0 { // 'retrying' es WARN, no INFO
			t.Errorf("q=retrying ∧ level=info no debe casar nada; got %q", out2)
		}
	})

	t.Run("tope de líneas", func(t *testing.T) {
		out := filterLogLines(strings.NewReader(sampleLog), "", "", 2)
		if lineCount(out) != 2 {
			t.Errorf("limit=2 debe devolver 2 líneas; got %q", out)
		}
		// El tope cuenta líneas CASADAS, no leídas: INFO está en la 1ª y 5ª línea.
		capped := filterLogLines(strings.NewReader(sampleLog), "", "info", 1)
		if lineCount(capped) != 1 || !strings.Contains(capped, "starting build") {
			t.Errorf("limit=1 con filtro debe devolver la 1ª casada; got %q", capped)
		}
	})
}
