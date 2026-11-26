// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package asistente

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NombreSistema es el nombre de la IA del sistema en Ajustes.
func NombreSistema() string { return "Apple Intelligence (por Atajos)" }

// EstadoDelSistema es el de la IA del sistema del MÓVIL: en el escritorio no hay estado
// que pintar (Ajustes la nombra por su sistema) y es SistemaComprobando.
func EstadoDelSistema() EstadoSistema { return SistemaComprobando }

// SistemaDisponible dice si el atajo existe.
func SistemaDisponible(ctx context.Context) bool {
	out, err := exec.CommandContext(ctx, "shortcuts", "list").Output()
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(l) == AtajoIA {
			return true
		}
	}
	return false
}

func completarSistema(ctx context.Context, _ Config, system, user string) (string, error) {
	dir, err := os.MkdirTemp("", "sytue-office-ia-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	entrada, salida := filepath.Join(dir, "entrada.txt"), filepath.Join(dir, "salida.txt")
	if err := os.WriteFile(entrada, []byte(system+"\n\n"+user), 0o600); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "shortcuts", "run", AtajoIA, "--input-path", entrada, "--output-path", salida)
	if out, err := cmd.CombinedOutput(); err != nil {
		if !SistemaDisponible(ctx) {
			return "", &ErrorEscritorio{Codigo: EscritorioFaltaAtajo}
		}
		return "", &ErrorEscritorio{Codigo: EscritorioFalloAtajo, Detalle: strings.TrimSpace(string(out))}
	}
	raw, err := os.ReadFile(salida)
	if err != nil {
		return "", err
	}
	texto := strings.TrimSpace(string(raw))
	if texto == "" {
		return "", &ErrorEscritorio{Codigo: EscritorioSinTexto}
	}
	return texto, nil
}
