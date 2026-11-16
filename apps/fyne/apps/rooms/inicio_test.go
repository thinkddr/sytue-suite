// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El inicio del canal arriba del historial (ROOM-13): `InicioDelCanal.test.tsx`.

var creadoEl = time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)

func canalConInicio() Canal {
	return Canal{
		ID: "c1", Nombre: "general", Tema: "Lanzamiento del viernes",
		Proposito: "Todo lo del equipo de producto", CreadoPor: "ana", Creado: creadoEl,
	}
}

// rotulosDeBotones son los textos de los botones que cuelgan de o, en orden.
func rotulosDeBotones(o fyne.CanvasObject) []string {
	bs := piezasEn[*ui.Boton](o)
	out := make([]string, 0, len(bs))
	for _, b := range bs {
		out = append(out, b.Texto)
	}
	return out
}

// primeraFila es lo primero de la columna del historial.
func primeraFila(t *testing.T, h *Historial) fyne.CanvasObject {
	t.Helper()
	s, ok := h.cuerpo.(*container.Scroll)
	if !ok {
		t.Fatalf("el historial no es un desplazador sino %T", h.cuerpo)
	}
	c, ok := s.Content.(*fyne.Container)
	if !ok || len(c.Objects) == 0 {
		t.Fatal("el historial no tiene filas")
	}
	return c.Objects[0]
}

func historialConInicio(t *testing.T, in *InicioCanal, hayAnteriores bool, ms ...Mensaje) *Historial {
	t.Helper()
	h := NuevoHistorial(traductor(t), "es")
	h.Ahora = func() time.Time { return creadoEl.Add(24 * time.Hour) }
	h.Estado, h.Mensajes, h.HayAnteriores, h.Inicio = Listo, ms, hayAnteriores, in
	if len(ms) == 0 {
		h.Estado = Vacio
	}
	w := test.NewTempWindow(t, h)
	w.Resize(fyne.NewSize(800, 600))
	h.Refresh()
	return h
}

func TestInicioArribaDelHistorialAlAgotarLaPaginacion(t *testing.T) {
	m := Mensaje{ID: "m1", Autor: Autor{ID: "beto", Nombre: "Beto"}, Cuerpo: "hola", Creado: creadoEl.Add(time.Hour)}
	in := &InicioCanal{Canal: canalConInicio(), Yo: "yo", NombreCreador: "Ana"}

	h := historialConInicio(t, in, false, m)
	arriba := strings.Join(textos(primeraFila(t, h)), "\n")
	for _, quiero := range []string{
		"Este es el principio de #general",
		"Ana creó este canal el 3 de mayo de 2026.",
		"Todo lo del equipo de producto",
		"Tema: Lanzamiento del viernes",
	} {
		if !strings.Contains(arriba, quiero) {
			t.Errorf("el inicio no dice %q; dice:\n%s", quiero, arriba)
		}
	}
	if strings.Contains(arriba, "Escribe el primer mensaje") {
		t.Error("con conversación, el inicio no invita a escribir el primero")
	}

	// Mientras quede una página anterior, lo de arriba no es el principio.
	h = historialConInicio(t, in, true, m)
	if dice(primeraFila(t, h), "Este es el principio de #general") {
		t.Error("con historial por pedir, arriba ya sale el inicio del canal")
	}
}

func TestInicioSustituyeAlVacio(t *testing.T) {
	in := &InicioCanal{Canal: canalConInicio(), Yo: "yo", NombreCreador: "Ana"}
	h := historialConInicio(t, in, false)
	if _, es := h.cuerpo.(*ui.Vacio); es {
		t.Fatal("con el inicio, el canal vacío sigue pintando el estado vacío de antes")
	}
	dentro := strings.Join(textos(primeraFila(t, h)), "\n")
	for _, quiero := range []string{"Este es el principio de #general", "Escribe el primer mensaje para empezar la conversación."} {
		if !strings.Contains(dentro, quiero) {
			t.Errorf("el canal vacío no dice %q; dice:\n%s", quiero, dentro)
		}
	}
}

func TestInicioAccionesSoloParaQuienPuede(t *testing.T) {
	tr := traductor(t)
	sin := NuevoBloqueInicio(tr, "es", InicioCanal{Canal: canalConInicio(), Yo: "yo"}, false)
	if b := rotulosDeBotones(sin); len(b) != 0 {
		t.Errorf("sin permisos el inicio ofrece %v", b)
	}
	anadir, editar := 0, 0
	con := NuevoBloqueInicio(tr, "es", InicioCanal{
		Canal: canalConInicio(), Yo: "yo",
		AlAnadirPersonas: func() { anadir++ }, AlEditarTema: func() { editar++ },
	}, false)
	if b := rotulosDeBotones(con); !slices.Equal(b, []string{"Añadir personas", "Editar el tema"}) {
		t.Errorf("las acciones son %v", b)
	}
	for _, b := range piezasEn[*ui.Boton](con) {
		test.Tap(b)
	}
	if anadir != 1 || editar != 1 {
		t.Errorf("pulsar las acciones dio añadir=%d editar=%d", anadir, editar)
	}
	c := canalConInicio()
	c.Tema = ""
	sinTema := NuevoBloqueInicio(tr, "es", InicioCanal{Canal: c, Yo: "yo", AlEditarTema: func() {}}, false)
	if b := rotulosDeBotones(sinTema); !slices.Equal(b, []string{"Añadir un tema"}) {
		t.Errorf("sin tema la acción es %v", b)
	}
}

func TestInicioQuienCreoElCanal(t *testing.T) {
	tr := traductor(t)
	c := canalConInicio()
	if s := LineaDeCreacion(tr, "es", c, "ana", ""); s != "Creaste este canal el 3 de mayo de 2026." {
		t.Errorf("para quien lo creó dice %q", s)
	}
	if s := LineaDeCreacion(tr, "es", c, "yo", ""); s != "Alguien creó este canal el 3 de mayo de 2026." {
		t.Errorf("sin nombre dice %q", s)
	}
	c.Creado = time.Time{}
	if s := LineaDeCreacion(tr, "es", c, "yo", "Ana"); s != "" {
		t.Errorf("sin fecha se inventa %q", s)
	}
}

func TestInicioDeUnDirecto(t *testing.T) {
	dm := Canal{ID: "d1", Nombre: "Ana", Clase: Directo, Presencia: ui.EnLinea}
	b := NuevoBloqueInicio(traductor(t), "es", InicioCanal{
		Canal: dm, Yo: "yo", AlAnadirPersonas: func() {}, AlEditarTema: func() {},
	}, false)
	dentro := strings.Join(textos(b), "\n")
	for _, quiero := range []string{"Conversación con Ana", "En línea", "Este es el principio de tu conversación directa con Ana."} {
		if !strings.Contains(dentro, quiero) {
			t.Errorf("el directo no dice %q; dice:\n%s", quiero, dentro)
		}
	}
	if r := rotulosDeBotones(b); len(r) != 0 {
		t.Errorf("un directo no tiene acciones de canal y ofrece %v", r)
	}
}

// En la Sala: el canal del servicio trae quién lo creó, el roster pone el nombre y
// «Añadir personas» abre el panel ya en «Añadir».
func TestInicioEnLaSala(t *testing.T) {
	s := servicioConDos()
	s.canales[0].CreatedBy, s.canales[0].CreatedAt = "ana", creadoEl
	s.roster = []api.RoomsMember{
		{Subject: "ana", Name: "Ana Ruiz"},
		{Subject: "yo", Name: "Yo", Role: "admin"},
	}
	sa, _ := salaDePrueba(t, s)
	sa.Arrancar(context.Background())

	h := sa.P.Canal.Historial
	if h.HayAnteriores || h.Inicio == nil {
		t.Fatalf("con cuatro mensajes no quedan anteriores (hay=%v) y tiene que haber inicio (%v)", h.HayAnteriores, h.Inicio)
	}
	arriba := primeraFila(t, h)
	if !strings.Contains(strings.Join(textos(arriba), "\n"), "Ana Ruiz creó este canal el 3 de mayo de 2026.") {
		t.Errorf("el inicio no dice quién creó el canal: %v", textos(arriba))
	}
	var anadir *ui.Boton
	for _, b := range piezasEn[*ui.Boton](arriba) {
		if b.Texto == "Añadir personas" {
			anadir = b
		}
	}
	if anadir == nil {
		t.Fatalf("quien administra el canal no ve «Añadir personas»: %v", rotulosDeBotones(arriba))
	}
	test.Tap(anadir)
	if sa.miembros == nil || !sa.miembros.Anadiendo {
		t.Error("«Añadir personas» no abre el panel de personas en «Añadir»")
	}
}
