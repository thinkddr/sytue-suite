// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// cuantas cuenta las llamadas del servicio que empiezan por ese prefijo.
func cuantas(s *servicioFalso, prefijo string) int {
	n := 0
	for _, l := range s.llamadas {
		if strings.HasPrefix(l, prefijo) {
			n++
		}
	}
	return n
}

// TestLosAtajosDeTriajeDeGmail: `Mayús+I`/`Mayús+U` marcan leída y no leída la abierta
// o la fila del cursor, «!» manda a spam y «z» deshace el último triaje —una vez—. Y
// los cuatro salen en la hoja de «?».
//
// VISTO EN ROJO (2026-11-13): sin los casos 'I', 'U', '!' y 'z' en `Atajo` ninguna
// tecla hace nada, y sin `ultimoTriaje` «z» no tiene qué deshacer.
func TestLosAtajosDeTriajeDeGmail(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	c := w.Canvas()
	tr := traductor(t)
	vistos := `SetFlagMany(\Seen)`

	b.Atajo(c, 'j') // abre la primera, que queda leída
	if b.Atajo(c, 'I') {
		t.Error("Mayús+I sobre una conversación ya leída no es atajo")
	}
	antes := cuantas(s, vistos)
	if !b.Atajo(c, 'U') || cuantas(s, vistos) != antes+1 || b.hiloID != "" {
		t.Fatalf("Mayús+U no marca no leída la abierta ni vuelve a la lista: %v", s.llamadas)
	}
	// Sin nada abierto, la fila del cursor: la misma, que ahora está sin leer.
	if !b.P.Bandeja.Conversaciones[0].SinLeer {
		t.Fatal("tras Mayús+U la conversación sigue leída en la lista")
	}
	if !b.Atajo(c, 'I') || cuantas(s, vistos) != antes+2 || b.P.Bandeja.Conversaciones[0].SinLeer {
		t.Errorf("Mayús+I no marca leída la fila del cursor: %v", s.llamadas)
	}

	if b.Atajo(c, 'z') {
		t.Error("«z» sin ningún triaje que deshacer no es atajo")
	}
	primera := b.P.Bandeja.Conversaciones[0]
	if !b.Atajo(c, '!') || !slices.ContainsFunc(s.llamadas, func(l string) bool { return strings.HasPrefix(l, "MoveMany(jk,") }) {
		t.Fatalf("«!» no manda a spam: %v", s.llamadas)
	}
	if !b.Atajo(c, 'z') || !slices.ContainsFunc(s.llamadas, func(l string) bool { return strings.HasPrefix(l, "MoveMany(in,") }) {
		t.Fatalf("«z» no devuelve %q a Recibidos: %v", primera.HiloID, s.llamadas)
	}
	if b.Atajo(c, 'z') {
		t.Error("«z» deshace dos veces el mismo triaje")
	}

	b.verAtajos()
	for _, clave := range []string{"shortcuts.markRead", "shortcuts.markUnread", "shortcuts.spam", "shortcuts.undo"} {
		if !hayTexto(c, tr.T(clave, nil)) {
			t.Errorf("la hoja de atajos no lista %q", clave)
		}
	}
}

// TestLosAtajosDeTriajeNoHacenNadaConUnaCapaNiEnSoloLectura: con un diálogo encima el
// teclado es suyo, y en un buzón de solo lectura no hay triaje que hacer ni deshacer.
func TestLosAtajosDeTriajeNoHacenNadaConUnaCapaNiEnSoloLectura(t *testing.T) {
	s := servicioConTodas()
	b, w, _ := buzonConVentana(t, s, fyne.NewSize(1280, 744))
	c := w.Canvas()
	b.Atajo(c, 'j')
	b.P.Acciones.Archivar(b.P.Bandeja.Conversaciones[1]) // deja algo que deshacer

	b.verAtajos()
	antes := len(s.llamadas)
	for _, r := range []rune{'I', 'U', '!', 'z'} {
		if b.Atajo(c, r) {
			t.Errorf("con la hoja abierta, %q ha sido atajo", r)
		}
	}
	ui.Descartar(c)

	b.P.Bandeja.SoloLectura = true
	for _, r := range []rune{'U', '!', 'z'} {
		if b.Atajo(c, r) {
			t.Errorf("en solo lectura, %q ha sido atajo", r)
		}
	}
	if len(s.llamadas) != antes {
		t.Errorf("los atajos han llamado al servidor: %v", s.llamadas[antes:])
	}
}
