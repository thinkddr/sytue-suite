// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"io"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestElPeriodoEsElDePeriodRange: mes y año empiezan a las 00:00 de QUIEN MIRA, y
// «Próximos» y «Pasados» parten de ahora; «Cualquier fecha» no acota.
func TestElPeriodoEsElDePeriodRange(t *testing.T) {
	madrid, _ := time.LoadLocation("Europe/Madrid")
	ahora := time.Date(2026, 12, 15, 10, 30, 0, 0, madrid)
	casos := map[string][2]time.Time{
		"any":      {},
		"upcoming": {ahora, {}},
		"past":     {{}, ahora},
		"month":    {time.Date(2026, 12, 1, 0, 0, 0, 0, madrid), time.Date(2027, 1, 1, 0, 0, 0, 0, madrid)},
		"year":     {time.Date(2026, 1, 1, 0, 0, 0, 0, madrid), time.Date(2027, 1, 1, 0, 0, 0, 0, madrid)},
	}
	for periodo, quiero := range casos {
		desde, hasta := rangoPeriodo(periodo, ahora)
		if !desde.Equal(quiero[0]) || !hasta.Equal(quiero[1]) {
			t.Errorf("%s: [%v, %v) y quiero [%v, %v)", periodo, desde, hasta, quiero[0], quiero[1])
		}
	}
	if len(PeriodosBusqueda) != len(casos) {
		t.Errorf("hay %d periodos y se prueban %d", len(PeriodosBusqueda), len(casos))
	}
}

// buscador es Sytue Calendar que, además, apunta cada búsqueda.
type buscador struct {
	escrituras
	mu       sync.Mutex
	consulta []url.Values
	// paginado: la primera página trae un resultado y un cursor a la segunda.
	paginado bool
}

func (b *buscador) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/events/search" {
		b.mu.Lock()
		b.consulta = append(b.consulta, r.URL.Query())
		b.mu.Unlock()
		if b.paginado && !r.URL.Query().Has("cursor") {
			_, _ = io.WriteString(w, `{"events":[{"id":"r1","calendar_id":"mio","uid":"r1@sytue.eu",`+
				`"summary":"Comité","dtstart":"2026-11-20T09:00:00Z","dtend":"2026-11-20T10:00:00Z"}],"next_cursor":"c-1"}`)
			return
		}
		_, _ = io.WriteString(w, `{"events":[],"next_cursor":""}`)
		return
	}
	b.escrituras.ServeHTTP(w, r)
}

// esperarBusqueda espera a la búsqueda número n (la espera de 250 ms va por un reloj).
func (b *buscador) esperarBusqueda(t *testing.T, n int) url.Values {
	t.Helper()
	for limite := time.Now().Add(3 * time.Second); time.Now().Before(limite); {
		fyne.DoAndWait(func() {})
		b.mu.Lock()
		if len(b.consulta) >= n {
			q := b.consulta[n-1]
			b.mu.Unlock()
			return q
		}
		b.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no llegó la búsqueda %d", n)
	return nil
}

// TestLaBusquedaFiltraPorCalendarioYPeriodo es SearchPanel.tsx: bajo el campo, el
// calendario (solo los propios) y el periodo; cambiarlos relanza la búsqueda con
// `calendar` y `from`/`to`.
func TestLaBusquedaFiltraPorCalendarioYPeriodo(t *testing.T) {
	b := &buscador{}
	a, w := appConVentana(t, b)
	// La respuesta llega por el reloj de la espera, en otra goroutine: se pinta con
	// fyne.Do, como en producción, y esperarBusqueda la vacía en el hilo de la prueba.
	a.enUI = fyne.Do
	a.abrirBusqueda()
	capa := w.Canvas().Overlays().Top()
	if capa == nil {
		t.Fatal("no se abrió el buscador")
	}
	selector := func(rotulo string) *ui.Selector {
		s, _ := buscarObjeto(capa, func(o fyne.CanvasObject) bool {
			x, ok := o.(*ui.Selector)
			return ok && x.Rotulo == rotulo
		}).(*ui.Selector)
		if s == nil {
			t.Fatalf("no está el filtro %q", rotulo)
		}
		return s
	}
	cal, per := selector("Calendario"), selector("Periodo")
	if got := cal.Opciones; len(got) != 2 || got[0] != "Todos los calendarios" || got[1] != "Personal" {
		t.Errorf("los calendarios del filtro son %v", got)
	}
	if per.Valor() != "Cualquier fecha" || len(per.Opciones) != len(PeriodosBusqueda) {
		t.Errorf("el periodo arranca en %q con %d opciones", per.Valor(), len(per.Opciones))
	}
	campo, _ := buscarObjeto(capa, func(o fyne.CanvasObject) bool { _, ok := o.(*ui.Entrada); return ok }).(*ui.Entrada)
	campo.PonerTexto("comité")
	if q := b.esperarBusqueda(t, 1); q.Has("calendar") || q.Has("from") || q.Has("to") {
		t.Errorf("sin filtros se pidió %v", q)
	}
	cal.Elegida = 1
	cal.AlElegir(1)
	if q := b.esperarBusqueda(t, 2); q.Get("calendar") != "mio" || q.Has("from") {
		t.Errorf("con el calendario se pidió %v", q)
	}
	per.Elegida = 1 // «Próximos»
	per.AlElegir(1)
	q := b.esperarBusqueda(t, 3)
	desde, err := time.Parse(time.RFC3339, q.Get("from"))
	if err != nil || time.Since(desde) > time.Minute || q.Has("to") || q.Get("calendar") != "mio" {
		t.Errorf("con «Próximos» se pidió %v", q)
	}
}

// TestCargarMasReutilizaElTramoDeLaPrimeraPagina: la paginación es keyset y el
// cursor solo vale con el tramo con el que se pidió la primera página. Con
// «Próximos», «Cargar más» recalculaba `from` con la hora de ahora, y un evento que
// empezaba entre una página y otra podía saltarse o salir dos veces.
func TestCargarMasReutilizaElTramoDeLaPrimeraPagina(t *testing.T) {
	b := &buscador{paginado: true}
	a, w := appConVentana(t, b)
	a.enUI = fyne.Do // como en producción: la respuesta no pinta desde el reloj
	a.abrirBusqueda()
	capa := w.Canvas().Overlays().Top()
	per, _ := buscarObjeto(capa, func(o fyne.CanvasObject) bool {
		x, ok := o.(*ui.Selector)
		return ok && x.Rotulo == "Periodo"
	}).(*ui.Selector)
	per.Elegida = 1 // «Próximos»
	campo, _ := buscarObjeto(capa, func(o fyne.CanvasObject) bool { _, ok := o.(*ui.Entrada); return ok }).(*ui.Entrada)
	campo.PonerTexto("comité")
	primera := b.esperarBusqueda(t, 1)
	if !primera.Has("from") {
		t.Fatalf("con «Próximos» se pidió %v", primera)
	}
	// El `from` va en segundos: que pase uno entero entre una página y otra.
	time.Sleep(1100 * time.Millisecond)
	var mas *ui.Boton
	for limite := time.Now().Add(3 * time.Second); mas == nil && time.Now().Before(limite); {
		fyne.DoAndWait(func() {})
		mas, _ = buscarObjeto(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) bool {
			x, ok := o.(*ui.Boton)
			return ok && x.Texto == "Cargar más"
		}).(*ui.Boton)
	}
	if mas == nil {
		t.Fatal("no salió «Cargar más»")
	}
	mas.Tapped(nil)
	segunda := b.esperarBusqueda(t, 2)
	if segunda.Get("cursor") != "c-1" || segunda.Get("from") != primera.Get("from") || segunda.Get("q") != "comité" {
		t.Errorf("la primera página pidió %v y la segunda %v", primera, segunda)
	}
}
