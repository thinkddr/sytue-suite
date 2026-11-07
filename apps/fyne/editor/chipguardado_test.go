// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El chip «Guardado hace 2 min» de la cabecera se pulsa (EditorHeader.tsx): abre el
// historial de versiones —el mismo menú que «Historial» de Colaboración— y, con el
// guardado en error, reintenta ya. Sin red, inerte.
//
// VISTO EN ROJO (2026-11-05): con el chip como texto suelto no había nada que pulsar; con
// `pulsarChip` abriendo siempre el historial, en error no se reintentaba.
func TestElChipDeGuardadoAbreElHistorialOReintenta(t *testing.T) {
	pedidas, reintentos := 0, 0
	ctx := &Contexto{
		Tipo: Documento, Zoom: 100, Estado: Listo, Guardado: Limpio, Titulo: "Acta",
		Versiones: func(listo func([]api.DocVersion)) {
			pedidas++
			listo([]api.DocVersion{{ID: "v1", Label: "Antes del cierre", CreatedAt: time.Now()}})
		},
		GuardarYa: func() { reintentos++ },
	}
	m := NuevoMarco(ctx, Genericas(), nil)
	w := test.NewTempWindow(t, m)
	w.Resize(fyne.NewSize(1280, 800))
	chip := m.cabecera.chip
	if chip.Inerte || chip.Rotulo != "header.save.saved · versions.history" {
		t.Fatalf("guardado, el chip: inerte %v, nombre %q", chip.Inerte, chip.Rotulo)
	}
	test.Tap(chip)
	if pedidas != 1 || reintentos != 0 || chip.menu == nil || !chip.menu.Abierto() {
		t.Fatalf("pulsar el chip guardado: %d historiales pedidos, %d reintentos, menú %v", pedidas, reintentos, chip.menu)
	}
	if tx := textos(chip.menu); len(tx) != 1 || !strings.HasPrefix(tx[0], "Antes del cierre — ") {
		t.Errorf("el historial no enseña la versión: %v", tx)
	}
	chip.menu.Cerrar()

	ctx.Guardado = FalloAlGuardar
	ctx.Refrescar()
	if chip.Texto != "header.save.error" || chip.Token != "danger" || chip.Rotulo != "header.save.error · header.save.retry" {
		t.Fatalf("en error, el chip dice %q (%s), nombre %q", chip.Texto, chip.Token, chip.Rotulo)
	}
	test.Tap(chip)
	if reintentos != 1 || pedidas != 1 {
		t.Errorf("pulsar el chip en error: %d reintentos, %d historiales", reintentos, pedidas)
	}

	ctx.SinRed = true
	ctx.Refrescar()
	test.Tap(chip)
	if !chip.Inerte || reintentos != 1 || pedidas != 1 {
		t.Errorf("sin red el chip no hace nada: inerte %v, %d reintentos, %d historiales", chip.Inerte, reintentos, pedidas)
	}
}
