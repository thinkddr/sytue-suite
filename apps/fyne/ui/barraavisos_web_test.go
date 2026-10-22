// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La campana como NotificationCenter.tsx, medido contra la web el 2026-09-25 (v3.6.23):
//   - título y cuerpo PARTEN en líneas (ListItem recibe nodos, no cadenas), no «…»;
//   - la cabecera es `px-3 pt-1.5 pb-2`: 31,5 sin sin-leer y 36,75 con el tic de 24,5;
//   - ni icono de producto en la fila ni campana en «Ver todas», que es un enlace.
//
// VISTO EN ROJO con el panel de antes: la fila medía una línea (49), la cabecera 38,5 y
// la fila llevaba `Delantero`.
func TestLaCampanaPintaComoLaWeb(t *testing.T) {
	a := nuevoAvisosBarra(traductorPrueba(t), nil)

	if h := a.cabecera(0).MinSize().Height; math.Abs(float64(h-36)) > 0.01 {
		t.Errorf("cabecera sin sin-leer: %.2f, la web 36", h)
	}
	if h := a.cabecera(1).MinSize().Height; math.Abs(float64(h-42)) > 0.01 {
		t.Errorf("cabecera con sin-leer: %.2f, la web 42", h)
	}

	largo := "Operaciones te ha mencionado en «Verificación v3.6.23 14:02:03»"
	f, _ := a.fila(api.HubNotification{ID: "n", Source: "docs", Title: largo, Body: "Prueba de mención @ops@corporate.sytue.eu", CreatedAt: time.Now()}).(*FilaLista)
	if f == nil {
		t.Fatal("la fila no es una FilaLista")
	}
	if f.Delantero != nil {
		t.Error("la fila lleva icono de producto; la web no")
	}
	w := test.NewWindow(f)
	defer w.Close()
	w.Resize(fyne.NewSize(AnchoAvisos, 200))
	if h := f.MinSize().Height; h <= MedidaFilaLista(true, 0) {
		t.Errorf("la fila mide %.2f: título y cuerpo en una línea cada uno, la web los parte", h)
	}
	for _, o := range test.WidgetRenderer(f).Objects() {
		if p, ok := o.(*Parrafo); ok {
			for _, l := range LineasDe(p, AnchoAvisos) {
				if strings.HasSuffix(l, "…") {
					t.Errorf("línea recortada con «…»: %q", l)
				}
			}
		}
	}

	for _, o := range test.LaidOutObjects(a.verTodas()) {
		if _, es := o.(*ElementoMenu); es {
			t.Error("«Ver todas» es un elemento de menú con campana; en la web es un enlace")
		}
	}
}
