// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package local

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// hayAppPara pregunta al escritorio si alguna app atiende el esquema: `xdg-open` no
// falla cuando no la hay, así que probar a abrir no lo dice. Sin `xdg-mime`, no se sabe.
func hayAppPara(esquema string) (hay, sabido bool) {
	if _, err := exec.LookPath("xdg-mime"); err != nil {
		return false, false
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelar()
	//nolint:gosec // G204: un programa fijo y el esquema como un argumento más, sin intérprete de órdenes
	out, err := exec.CommandContext(ctx, "xdg-mime", "query", "default", "x-scheme-handler/"+esquema).Output()
	if err != nil {
		return false, true
	}
	return strings.TrimSpace(string(out)) != "", true
}
