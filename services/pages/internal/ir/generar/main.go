// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func main() {
	b, err := ir.GenerarEsquemaIA()
	if err != nil {
		slog.Error("generar esquema_ia", "err", err)
		os.Exit(1)
	}
	destino := "esquema_ia.json"
	if _, err := os.Stat(filepath.Join("services", "pages", "internal", "ir")); err == nil {
		destino = filepath.Join("services", "pages", "internal", "ir", "esquema_ia.json")
	}
	if err := os.WriteFile(destino, b, 0o600); err != nil {
		slog.Error("escribir esquema_ia", "destino", destino, "err", err)
		os.Exit(1)
	}
	slog.Info("esquema_ia.json generado con éxito", "destino", destino, "bytes", len(b))
}
