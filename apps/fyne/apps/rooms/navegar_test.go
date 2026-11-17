// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Los mismos canales que `lib/navegarCanales.test.ts`: el orden de entrada da igual.
func canalesNavegar() ([]Canal, []Categoria) {
	cats := []Categoria{{ID: "cat", Nombre: "Proyectos"}}
	return []Canal{
		{ID: "dm-ana", Nombre: "dm-ana", Clase: Directo, SinLeer: 2},
		{ID: "general", Nombre: "general"},
		{ID: "diseño", Nombre: "diseño", CategoriaID: "cat", SinLeer: 1, Silenciado: true},
		{ID: "avisos", Nombre: "avisos", SinLeer: 4},
		{ID: "equipo", Nombre: "equipo", Favorito: true},
		{ID: "obra", Nombre: "obra", CategoriaID: "cat", Menciones: 1, SinLeer: 3, Nivel: AvisarMenciones},
	}, cats
}

func ids(l []Canal) []string {
	out := make([]string, len(l))
	for i, c := range l {
		out[i] = c.ID
	}
	return out
}

func TestNavegaPorElOrdenVisibleDelCarril(t *testing.T) {
	canales, cats := canalesNavegar()
	secciones := AgruparCanales(canales, cats)
	nada := func(Seccion) bool { return false }
	if got, want := ids(OrdenVisible(secciones, nada)),
		[]string{"equipo", "avisos", "general", "diseño", "obra", "dm-ana"}; !slices.Equal(got, want) {
		t.Errorf("orden: %v, quiero %v", got, want)
	}
	plegadas := func(s Seccion) bool { return s.Clave == ClaveFavoritos || s.Clave == "cat" }
	if got, want := ids(OrdenVisible(secciones, plegadas)),
		[]string{"avisos", "general", "dm-ana"}; !slices.Equal(got, want) {
		t.Errorf("con plegadas: %v, quiero %v", got, want)
	}

	orden := OrdenVisible(secciones, nada)
	casos := []struct {
		desde   string
		paso    int
		solo    bool
		destino string // "" es que no se mueve
	}{
		{"avisos", 1, false, "general"},
		{"avisos", -1, false, "equipo"},
		{"equipo", -1, false, ""}, // a secas no da la vuelta
		{"dm-ana", 1, false, ""},
		{"", 1, false, "equipo"},  // sin abierto: el primero al bajar…
		{"", -1, false, "dm-ana"}, // …y el último al subir
		{"oculto", 1, false, "equipo"},
		// Con Mayús: el siguiente con algo pendiente; «diseño» está silenciado.
		{"equipo", 1, true, "avisos"},
		{"avisos", 1, true, "obra"},
		{"obra", 1, true, "dm-ana"},
		{"dm-ana", 1, true, "avisos"}, // da la vuelta
		{"avisos", -1, true, "dm-ana"},
		{"general", -1, true, "avisos"},
		{"", -1, true, "dm-ana"},
	}
	for _, c := range casos {
		got, ok := CanalVecino(orden, c.desde, c.paso, c.solo)
		if !ok {
			got.ID = ""
		}
		if got.ID != c.destino {
			t.Errorf("desde %q paso %d (pendientes %v): %q, quiero %q", c.desde, c.paso, c.solo, got.ID, c.destino)
		}
	}
	if _, ok := CanalVecino([]Canal{{ID: "a", SinLeer: 5}, {ID: "b"}}, "a", 1, true); ok {
		t.Error("el abierto no cuenta como pendiente")
	}
	if _, ok := CanalVecino(nil, "", 1, false); ok {
		t.Error("sin canales no hay adónde ir")
	}
}

func TestNavegaSoloConAltYSinCtrlNiSuper(t *testing.T) {
	atajo := func(k fyne.KeyName, m fyne.KeyModifier) fyne.Shortcut {
		return &desktop.CustomShortcut{KeyName: k, Modifier: m}
	}
	casos := []struct {
		s          fyne.Shortcut
		paso       int
		solo, vale bool
	}{
		{atajo(fyne.KeyDown, fyne.KeyModifierAlt), 1, false, true},
		{atajo(fyne.KeyUp, fyne.KeyModifierAlt|fyne.KeyModifierShift), -1, true, true},
		{atajo(fyne.KeyUp, fyne.KeyModifierShift), 0, false, false},
		{atajo(fyne.KeyUp, fyne.KeyModifierAlt|fyne.KeyModifierControl), 0, false, false},
		{atajo(fyne.KeyDown, fyne.KeyModifierAlt|fyne.KeyModifierSuper), 0, false, false},
		{atajo(fyne.KeyLeft, fyne.KeyModifierAlt), 0, false, false},
		{&fyne.ShortcutCopy{}, 0, false, false},
	}
	for i, c := range casos {
		paso, solo, ok := PasoDeNavegacion(c.s)
		if ok != c.vale || paso != c.paso || solo != c.solo {
			t.Errorf("caso %d: (%d, %v, %v), quiero (%d, %v, %v)", i, paso, solo, ok, c.paso, c.solo, c.vale)
		}
	}
}

// En la sala: Alt+↓ desde el lienzo abre el siguiente canal; en la caja solo con ella
// vacía (con texto, ⌥+flecha mueve el cursor en macOS), y con un diálogo delante no.
func TestNavegaEntreCanalesDeLaSala(t *testing.T) {
	sa, _, w := salaEn(t, servicioConDos(), 1280)
	sa.Atajos(w.Canvas())
	sa.Arrancar(context.Background())
	defer sa.Parar()
	c := w.Canvas()
	abajo := &desktop.CustomShortcut{KeyName: fyne.KeyDown, Modifier: fyne.KeyModifierAlt}
	arriba := &desktop.CustomShortcut{KeyName: fyne.KeyUp, Modifier: fyne.KeyModifierAlt}

	if sa.P.Lateral.CanalID != "c1" {
		t.Fatalf("abre %q y tenía que abrir «equipo»", sa.P.Lateral.CanalID)
	}
	if !sa.AtajoNavegar(c, abajo) || sa.P.Lateral.CanalID != "d1" {
		t.Fatalf("Alt+↓ desde el lienzo: abierto %q, quiero el directo", sa.P.Lateral.CanalID)
	}
	if sa.AtajoNavegar(c, abajo) {
		t.Error("Alt+↓ en el último canal no da la vuelta")
	}

	// En la caja: con texto la flecha es de quien escribe.
	caja := sa.P.Canal.Compositor
	caja.PonerTexto("hola")
	caja.area.TypedShortcut(arriba)
	if sa.P.Lateral.CanalID != "d1" {
		t.Fatalf("Alt+↑ con texto en la caja ha cambiado de canal a %q", sa.P.Lateral.CanalID)
	}
	caja.PonerTexto("")
	caja.area.TypedShortcut(arriba)
	if sa.P.Lateral.CanalID != "c1" {
		t.Fatalf("Alt+↑ con la caja vacía: abierto %q, quiero «equipo»", sa.P.Lateral.CanalID)
	}

	// Con la hoja de atajos delante el teclado es suyo.
	d := sa.VerAtajos(c)
	if sa.AtajoNavegar(c, abajo) || sa.P.Lateral.CanalID != "c1" {
		t.Errorf("con un diálogo abierto Alt+↓ ha cambiado de canal a %q", sa.P.Lateral.CanalID)
	}
	d.Cerrar()

	// Alt+Mayús+↓ salta al siguiente con algo pendiente: el directo los tiene.
	sa.pend["d1"] = api.RoomsUnread{ChannelID: "d1", Unread: 2}
	sa.pintarCanales()
	solo := &desktop.CustomShortcut{KeyName: fyne.KeyDown, Modifier: fyne.KeyModifierAlt | fyne.KeyModifierShift}
	if !sa.AtajoNavegar(c, solo) || sa.P.Lateral.CanalID != "d1" {
		t.Errorf("Alt+Mayús+↓: abierto %q, quiero el directo con pendientes", sa.P.Lateral.CanalID)
	}
}
