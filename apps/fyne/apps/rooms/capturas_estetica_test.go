// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido/montaje"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestCapturasEsteticas deja con SYTUE_CAPTURAS el canal abierto en las ocho
// combinaciones de la auditoría del 2026-11-24 (ver `montaje.Capturar`), el inicio de un
// canal sin mensajes y un canal algo más largo que la ventana.
func TestCapturasEsteticas(t *testing.T) {
	sa, _, w := salaEn(t, servicioConDos(), 1280)
	sa.Arrancar(context.Background())
	montaje.Capturar(t, w, "rooms-canal", iconos.Rooms)

	s := servicioConDos()
	s.mensajes = nil
	vacio, _, wv := salaEn(t, s, 1280)
	vacio.Arrancar(context.Background())
	montaje.Capturar(t, wv, "rooms-inicio", iconos.Rooms)

	// Un canal algo más largo que la ventana, abierto abajo: el inicio queda a medias
	// bajo la cabecera.
	l := servicioConDos()
	base := l.mensajes["c1"][0].CreatedAt
	l.pend = nil
	for i := range 9 {
		l.mensajes["c1"] = append(l.mensajes["c1"], api.RoomsMessage{
			ID: fmt.Sprintf("x%d", i), AuthorSub: "ana", AuthorName: "Ana",
			Body: "Mensaje de relleno para llenar el canal", CreatedAt: base.Add(time.Duration(10+i) * time.Minute),
		})
	}
	largo, _, wl := salaEn(t, l, 1280)
	largo.Arrancar(context.Background())
	montaje.Capturar(t, wl, "rooms-largo", iconos.Rooms)
}

// TestCapturasDeLaLlamada deja con SYTUE_CAPTURAS lo de la llamada que el plan del
// 2026-11-08 no pudo mirar: ROOM-03 y ROOM-14.
func TestCapturasDeLaLlamada(t *testing.T) {
	// ROOM-03: el punto de «hay llamada» en el carril y la franja «Llamada en curso · N».
	ll, _, wll := salaEn(t, servicioConDos(), 1280)
	ll.Arrancar(context.Background())
	ll.alEmpezarOAcabarLlamada("c1", true)
	ll.ponerLlamadaActiva(3)
	montaje.Capturar(t, wll, "rooms-llamada", iconos.Rooms, func(montaje.MedidaCaptura) {
		ll.P.Refrescar()
	})
}

// TestCapturaDeLaLlamadaSola: ROOM-14, la ventana de la llamada sin nadie más.
func TestCapturaDeLaLlamadaSola(t *testing.T) {
	// ROOM-14: la ventana de la llamada sin nadie más, con «Pantalla completa» en su menú.
	solo := &enCursoFalsa{ps: []llamada.Participante{{Identidad: "yo", Nombre: "Yo", Yo: true}}}
	v := NuevaVistaLlamada(traductor(t), "Guardia", solo, true)
	wv2 := test.NewTempWindow(t, v)
	wv2.Resize(fyne.NewSize(1280, 720))
	montaje.Capturar(t, wv2, "rooms-llamada-sola", iconos.Rooms)
}
