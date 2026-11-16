// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func TestLlegadosAbajoRecientes(t *testing.T) {
	m := func(id string, mio bool) Mensaje { return Mensaje{ID: id, Mio: mio} }
	casos := []struct {
		nombre string
		ms     []Mensaje
		antes  []string
		ids    []string
		ok     bool
	}{
		{"lo ajeno detrás del último", []Mensaje{m("a", false), m("b", false), m("c", false), m("d", false)}, []string{"a", "b"}, []string{"c", "d"}, true},
		{"lo propio no cuenta", []Mensaje{m("a", false), m("b", true), m("c", false)}, []string{"a"}, []string{"c"}, true},
		{"prepend de historial", []Mensaje{m("z", false), m("a", false), m("b", false)}, []string{"a", "b"}, nil, true},
		{"lista capada", []Mensaje{m("b", false), m("c", false), m("d", false)}, []string{"a", "b", "c"}, []string{"d"}, true},
		{"otra lista", []Mensaje{m("x", false)}, []string{"a", "b"}, nil, false},
		{"borrar el último no tira la cuenta", []Mensaje{m("a", false), m("c", false)}, []string{"a", "c", "d"}, nil, true},
		{"sin lista anterior", []Mensaje{m("a", false)}, nil, nil, true},
	}
	for _, c := range casos {
		ids, ok := LlegadosAbajo(c.ms, c.antes)
		if ok != c.ok || !slices.Equal(ids, c.ids) {
			t.Errorf("%s: %v, %v; quiero %v, %v", c.nombre, ids, ok, c.ids, c.ok)
		}
	}
	pendiente := []Mensaje{m("a", false), {ID: "tmp", Enviando: true}}
	if ids, _ := LlegadosAbajo(pendiente, []string{"a"}); len(ids) != 0 {
		t.Errorf("lo que va de camino cuenta: %v", ids)
	}
}

// historialLargo es un canal con más mensajes de los que caben, ya pintado.
func historialLargo(t *testing.T) (*Historial, fyne.Window) {
	t.Helper()
	h := NuevoHistorial(traductor(t), "es")
	h.Estado = Listo
	base := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h.Ahora = func() time.Time { return base.Add(time.Hour) }
	for i := range 30 {
		h.Mensajes = append(h.Mensajes, Mensaje{
			ID: fmt.Sprint(i), Autor: Autor{ID: "ana", Nombre: "Ana"}, Creado: base.Add(time.Duration(i) * time.Minute),
			Cuerpo: "línea",
		})
	}
	w := test.NewTempWindow(t, h)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(480, 300))
	return h, w
}

func llega(h *Historial, id string, mio bool) {
	ultimo := h.Mensajes[len(h.Mensajes)-1].Creado
	h.Mensajes = append(h.Mensajes, Mensaje{
		ID: id, Autor: Autor{ID: "ana", Nombre: "Ana"}, Creado: ultimo.Add(time.Minute), Cuerpo: "nuevo", Mio: mio,
	})
	h.Refresh()
}

func pildoraNuevos(h *Historial) bool {
	return slices.Contains(h.objetos, fyne.CanvasObject(h.bajar)) && h.bajar.Visible()
}

func TestNuevosAbajoRecientesConElScrollArriba(t *testing.T) {
	h, _ := historialLargo(t)
	s := scrollHistorial(t, h)
	if !alFondo(s) || pildoraNuevos(h) {
		t.Fatal("el canal no se abre por el final o abre con la píldora")
	}
	s.ScrollToTop()
	llega(h, "a1", false)
	llega(h, "mio", true)
	llega(h, "a2", false)
	if !pildoraNuevos(h) || h.NuevosAbajo() != 2 {
		t.Fatalf("con el scroll arriba, dos ajenos y uno propio: píldora %v con %d", pildoraNuevos(h), h.NuevosAbajo())
	}
	if want := h.T.T("message.newBelow", i18n.Vars{"count": 2}); h.bajar.Texto != want || want == "message.newBelow" {
		t.Errorf("la píldora dice %q y quiero %q", h.bajar.Texto, want)
	}
	if h.bajar.Variante != ui.Secundario || h.bajar.Talla != ui.TallaSM {
		t.Errorf("la píldora va en variante %d y talla %d", h.bajar.Variante, h.bajar.Talla)
	}
	b := h.bajar.Position()
	if b.Y+h.bajar.Size().Height != h.Size().Height-tema.Espacio(2) || b.X+h.bajar.Size().Width/2 != h.Size().Width/2 {
		t.Errorf("la píldora está en %v y va abajo y centrada (%v)", b, h.Size())
	}
	s = scrollHistorial(t, h)
	if alFondo(s) {
		t.Error("llegar algo bajó la vista de quien leía más arriba")
	}

	test.Tap(h.bajar)
	if !alFondo(scrollHistorial(t, h)) || pildoraNuevos(h) || h.NuevosAbajo() != 0 {
		t.Errorf("pulsarla no bajó al final o no la retiró: fondo %v, píldora %v", alFondo(scrollHistorial(t, h)), pildoraNuevos(h))
	}
	llega(h, "a3", false) // ya abajo: la vista lo sigue y no hay aviso
	if pildoraNuevos(h) || !alFondo(scrollHistorial(t, h)) {
		t.Error("estando abajo, lo nuevo dio píldora o no se siguió")
	}
}

func TestNuevosAbajoRecientesSeRetiraAlBajarAMano(t *testing.T) {
	h, _ := historialLargo(t)
	s := scrollHistorial(t, h)
	s.ScrollToTop()
	llega(h, "a1", false)
	if !pildoraNuevos(h) {
		t.Fatal("no aparece la píldora")
	}
	s = scrollHistorial(t, h)
	s.ScrollToBottom()
	s.OnScrolled(s.Offset)
	if pildoraNuevos(h) || h.NuevosAbajo() != 0 {
		t.Error("bajar a mano hasta el final no la retira")
	}
}

func TestNuevosAbajoRecientesOtraListaReinicia(t *testing.T) {
	h, _ := historialLargo(t)
	scrollHistorial(t, h).ScrollToTop()
	llega(h, "a1", false)
	h.Mensajes = []Mensaje{{ID: "x", Autor: Autor{ID: "ana", Nombre: "Ana"}, Creado: time.Now(), Cuerpo: "otro canal"}}
	h.Refresh()
	if pildoraNuevos(h) {
		t.Error("cambiar de lista arrastra la cuenta de la anterior")
	}
}

func TestNuevosAbajoRecientesBorrarElUltimoNoTiraLaCuenta(t *testing.T) {
	h, _ := historialLargo(t)
	scrollHistorial(t, h).ScrollToTop()
	llega(h, "a1", false)
	llega(h, "a2", false)
	h.Mensajes = h.Mensajes[:len(h.Mensajes)-1]
	h.Refresh()
	if !pildoraNuevos(h) || h.NuevosAbajo() != 1 {
		t.Errorf("borrar el último de dos nuevos deja la píldora %v con %d", pildoraNuevos(h), h.NuevosAbajo())
	}
}
