// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLoDesplazadoBajoLaCabeceraSeFunde: con el canal algo más largo que la ventana y
// abierto abajo, lo de arriba (el inicio del canal y su ilustración) queda a medias bajo
// la cabecera; el fundido de arriba lo funde con el papel. Arriba del todo no hay nada
// debajo y no se pinta. En escritorio Carril quedaba un retal de la ilustración pegado a
// una cabecera casi del color del papel (auditoría del 2026-11-24).
func TestLoDesplazadoBajoLaCabeceraSeFunde(t *testing.T) {
	s := servicioConDos()
	s.pend = nil
	base := s.mensajes["c1"][0].CreatedAt
	for i := range 9 {
		s.mensajes["c1"] = append(s.mensajes["c1"], api.RoomsMessage{
			ID: fmt.Sprintf("x%d", i), AuthorSub: "ana", AuthorName: "Ana",
			Body: "Mensaje de relleno", CreatedAt: base.Add(time.Duration(10+i) * time.Minute),
		})
	}
	sa, _, w := salaEn(t, s, 1280)
	w.Resize(fyne.NewSize(1280, 500))
	sa.Arrancar(context.Background())
	h := sa.P.Canal.Historial
	sc, es := h.cuerpo.(*container.Scroll)
	if !es {
		t.Fatalf("el historial no desplaza: %T", h.cuerpo)
	}
	if sc.Offset.Y <= 0 || !h.velo.Visible() {
		t.Fatalf("abierto abajo (desplazado %v) el fundido de arriba se ve %v", sc.Offset.Y, h.velo.Visible())
	}
	if h.velo.Position().Y != 0 || h.velo.Size().Height != AltoVeloHistorial || h.velo.Size().Width != h.Size().Width {
		t.Errorf("el fundido está en %v y mide %v", h.velo.Position(), h.velo.Size())
	}
	sc.ScrollToTop()
	h.alDesplazar(sc, sc.Offset)
	if h.velo.Visible() {
		t.Error("arriba del todo no hay nada bajo la cabecera y el fundido sigue")
	}
}
