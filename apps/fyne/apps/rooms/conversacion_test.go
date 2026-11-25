// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestConversacionEnTexto: el mismo texto que `conversacionEnTexto` de la web, una línea
// por mensaje con su hora local y su autor, y lo de varias líneas sangrado.
func TestConversacionEnTexto(t *testing.T) {
	a := time.Date(2026, 11, 25, 9, 5, 0, 0, time.Local)
	texto := ConversacionEnTexto("general", []api.RoomsMessage{
		{AuthorName: "Ana", Body: "Hola", CreatedAt: a},
		{AuthorName: "Luis", Body: "Dos\nlíneas", CreatedAt: a.Add(2 * time.Minute)},
	})
	quiero := "# general\n\n[2026-11-25 09:05] Ana: Hola\n[2026-11-25 09:07] Luis: Dos\n    líneas\n"
	if texto != quiero {
		t.Errorf("ConversacionEnTexto =\n%q\nquiero\n%q", texto, quiero)
	}
	if got := NombreDeConversacion("ventas/norte: Q4", a); got != "ventas norte Q4 2026-11-25.txt" {
		t.Errorf("NombreDeConversacion = %q", got)
	}
	if got := NombreDeConversacion("///", a); got != "rooms 2026-11-25.txt" {
		t.Errorf("sin nombre utilizable: %q", got)
	}
}

// TestElRechazoPorLaPoliticaDeFueraTieneSuTexto: `rooms_external_closed` dice que la
// organización no deja hablar con esa persona, en el panel de personas y al abrir un
// directo; un 403 cualquiera sigue siendo «no tienes permiso».
func TestElRechazoPorLaPoliticaDeFueraTieneSuTexto(t *testing.T) {
	cerrado := &api.Error{Status: 403, Body: `{"code":"rooms_external_closed","message":"x"}`}
	if !CerradoAFuera(cerrado) || ClaveErrorMiembros(cerrado) != "dm.externalClosed" {
		t.Errorf("rooms_external_closed: cerrado=%v clave=%q", CerradoAFuera(cerrado), ClaveErrorMiembros(cerrado))
	}
	otro := &api.Error{Status: 403, Body: `{"code":"forbidden"}`}
	if CerradoAFuera(otro) || ClaveErrorMiembros(otro) != "members.errorForbidden" {
		t.Errorf("403 corriente: cerrado=%v clave=%q", CerradoAFuera(otro), ClaveErrorMiembros(otro))
	}
	if CerradoAFuera(errors.New("red")) {
		t.Error("un fallo de red no es la política de fuera")
	}
}

// TestMarcarComoLeidoDesdeLaFila: con algo pendiente, la fila ofrece «Marcar como leído»
// lo primero; pulsarlo pone el contador a cero ya y lo pide al servicio. Sin nada
// pendiente no se ofrece (lo comprueba TestElMenuDeLaFilaDelCanal).
func TestMarcarComoLeidoDesdeLaFila(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())
	c := sa.P.Lateral.Canales[1]
	sa.pend[c.ID] = api.RoomsUnread{ChannelID: c.ID, Unread: 3, Mentions: 1}
	sa.MenuFila(c, sa.P.Canal.Cabecera)
	primero, ok := sa.menuFila.Elementos[0].(*ui.ElementoMenu)
	if !ok || primero.Texto != sa.P.T.T("channel.markRead", nil) {
		t.Fatalf("lo primero del menú no es «Marcar como leído»: %+v", sa.menuFila.Elementos[0])
	}
	primero.AlPulsar()
	if _, sigue := sa.pend[c.ID]; sigue {
		t.Error("el contador del canal sigue pendiente")
	}
	if !s.pidio("MarkRead(" + c.ID + ")") {
		t.Errorf("no se pidió al servicio: %v", s.llamadas)
	}
}
