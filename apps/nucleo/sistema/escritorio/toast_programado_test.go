// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import (
	"strings"
	"testing"
	"time"
)

// El título de un evento puede venir de un .ics ajeno: ni sus comillas ni un `$(…)`
// llegan a PowerShell como órdenes, y la etiqueta solo lleva lo que admite.
func TestElToastProgramadoVaEscapado(t *testing.T) {
	titulo := `Reunión ’$(Remove-Item C:\ -Recurse)' "x" <b>`
	g := guionToastProgramado("eu.sytue.office.calendar", "sytueoffice-calendar:/abrir?evento=1&x=2", titulo, "l’après", "ab12'; rm", time.Date(2026, 11, 23, 9, 30, 0, 0, time.UTC))
	for _, linea := range strings.Split(g, "\n") {
		// Cada cadena literal de PowerShell va entre comillas rectas, y dentro no queda
		// ninguna comilla simple que la cierre antes de tiempo.
		if i := strings.Index(linea, "('"); i >= 0 {
			dentro := linea[i+2 : strings.LastIndex(linea, "')")]
			if strings.ContainsAny(strings.ReplaceAll(dentro, "''", ""), "'‘’‚‛") {
				t.Errorf("comilla sin escapar en %q", linea)
			}
		}
	}
	if !strings.Contains(g, "$s.Tag = 'ab12rm'") {
		t.Errorf("etiqueta: %s", g)
	}
	if !strings.Contains(g, "2026-11-23T09:30:00Z") || !strings.Contains(g, "&amp;x=2") {
		t.Errorf("hora o enlace: %s", g)
	}
	if !strings.Contains(guionCancelarToast("eu.sytue.office.calendar", "ab12"), "-eq 'ab12'") {
		t.Error("cancelar no busca la etiqueta")
	}
}
