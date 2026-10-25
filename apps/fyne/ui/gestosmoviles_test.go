// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func TestGestoRevisionListaValidaLimitesFirmados(t *testing.T) {
	for _, tc := range []struct {
		sequence int
		want     uint64
	}{
		{math.MinInt, 0}, {-1, 0}, {0, 0}, {1, 1}, {math.MaxInt, uint64(math.MaxInt)},
	} {
		if got := RevisionLista(tc.sequence); got != tc.want {
			t.Fatalf("sequence %d: got %d, want %d", tc.sequence, got, tc.want)
		}
	}
}

func listaDeGesto(t *testing.T, options *RefrescoLista) *listaActualizable {
	t.Helper()
	previous := esMovil
	esMovil = func() bool { return true }
	t.Cleanup(func() { esMovil = previous })
	w, ok := RefrescarAlTirar(widget.NewLabel("fila"), options).(*listaActualizable)
	if !ok {
		t.Fatal("mobile list was not registered")
	}
	return w
}

func eventoGesto(phase mobile.FilterPhase, x, y float32, path ...fyne.CanvasObject) mobile.FilterEvent {
	return mobile.FilterEvent{Phase: phase, ID: 1, Position: fyne.NewPos(x, y), Time: time.Unix(100, 0), Path: path}
}

func comprobarFiltro(t *testing.T, g *gestosMoviles, e mobile.FilterEvent, want mobile.FilterResult) {
	t.Helper()
	if got := g.recibir(e); got != want {
		t.Fatalf("phase %v at %v: got %v, want %v", e.Phase, e.Position, got, want)
	}
}

func TestGestoListaNoCambiaElContenidoDeEscritorioNiInterceptaControles(t *testing.T) {
	previous := esMovil
	t.Cleanup(func() { esMovil = previous })
	esMovil = func() bool { return false }
	content := widget.NewLabel("fila")
	if RefrescarAlTirar(content, nil) != content || conGestosMoviles(content) != content {
		t.Fatal("desktop content must retain its identity")
	}
	l := listaDeGesto(t, nil)
	for _, object := range []fyne.CanvasObject{l, conGestosMoviles(l)} {
		if _, intercepts := object.(fyne.Tappable); intercepts {
			t.Fatal("gesture marker intercepts taps")
		}
		if _, intercepts := object.(fyne.Draggable); intercepts {
			t.Fatal("gesture marker intercepts scrolling or editors")
		}
		if _, intercepts := object.(mobile.Touchable); intercepts {
			t.Fatal("gesture marker intercepts touch delivery")
		}
	}
}

func TestGestoRaizConservaElArmazonParaAtrasYMarco(t *testing.T) {
	previous := esMovil
	esMovil = func() bool { return true }
	t.Cleanup(func() { esMovil = previous })
	a := NuevoArmazon(widget.NewLabel("detail"))
	root := conGestosMoviles(a)
	if armazonEn(root) != a {
		t.Fatal("mobile gesture root hid the app shell from Back and appearance")
	}
}

func TestGestoActualizarSoloAlSoltarYSinRepetir(t *testing.T) {
	calls := 0
	l := listaDeGesto(t, &RefrescoLista{Arriba: func() bool { return true }, Actualizar: func() { calls++ }})
	g := &gestosMoviles{}
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 100, 100, l), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 102, 180), mobile.FilterClaim)
	if calls != 0 {
		t.Fatal("refresh fired before release")
	}
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 102, 180, l), mobile.FilterClaim)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 102, 180, l), mobile.FilterPass)
	if calls != 1 {
		t.Fatalf("one pull performed %d refreshes", calls)
	}
}

func TestGestoListaDeclinaScrollFormulariosYMovimientosSinIntencion(t *testing.T) {
	for _, tc := range []struct {
		name       string
		top, ready bool
		x, y       float32
		form       bool
	}{
		{name: "scroll", ready: true, y: 100},
		{name: "busy", top: true, y: 100},
		{name: "small", top: true, ready: true, y: 8},
		{name: "upwards", top: true, ready: true, y: -90},
		{name: "horizontal", top: true, ready: true, x: 100},
		{name: "diagonal", top: true, ready: true, x: 80, y: 80},
		{name: "form", top: true, ready: true, y: 100, form: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			l := listaDeGesto(t, &RefrescoLista{Arriba: func() bool { return tc.top }, Puede: func() bool { return tc.ready }, Actualizar: func() { calls++ }})
			path := []fyne.CanvasObject{l}
			if tc.form {
				path = append(path, widget.NewEntry())
			}
			g := &gestosMoviles{}
			begin := g.recibir(eventoGesto(mobile.FilterBegin, 100, 200, path...))
			if !tc.top || !tc.ready || tc.form {
				if begin != mobile.FilterPass {
					t.Fatal("ordinary control was deferred")
				}
			} else {
				comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 100+tc.x, 200+tc.y, path...), mobile.FilterPass)
			}
			if calls != 0 {
				t.Fatal("unintended gesture refreshed")
			}
		})
	}
}

func TestGestoListaCapturadaRevalidaPermisosVistaYCancelacion(t *testing.T) {
	for _, reason := range []string{"busy", "scrolled", "revision", "replaced", "cancel", "multitouch"} {
		t.Run(reason, func(t *testing.T) {
			calls, revision := 0, uint64(1)
			top, ready := true, true
			l := listaDeGesto(t, &RefrescoLista{
				Arriba: func() bool { return top }, Puede: func() bool { return ready },
				Revision: func() uint64 { return revision }, Actualizar: func() { calls++ },
			})
			g := &gestosMoviles{}
			comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 100, 100, l), mobile.FilterDefer)
			comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 100, 180), mobile.FilterClaim)
			path := []fyne.CanvasObject{l}
			switch reason {
			case "busy":
				ready = false
			case "scrolled":
				top = false
			case "revision":
				revision++
			case "replaced":
				path = nil
			case "cancel", "multitouch":
				// The canvas sends Cancel before replaying a pending sequence or
				// suppressing a claimed multi-touch sequence until all fingers lift.
				e := eventoGesto(mobile.FilterCancel, 100, 180)
				if reason == "multitouch" {
					e.ID = 2
				}
				comprobarFiltro(t, g, e, mobile.FilterPass)
			}
			g.recibir(eventoGesto(mobile.FilterEnd, 100, 180, path...))
			if calls != 0 {
				t.Fatal("invalidated pull refreshed")
			}
		})
	}
}

func TestGestoPendienteNoReproduceToqueEnUnaVistaDistinta(t *testing.T) {
	l := listaDeGesto(t, &RefrescoLista{Arriba: func() bool { return true }, Actualizar: func() { t.Fatal("unexpected refresh") }})
	g := &gestosMoviles{}
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 100, 100, l), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 101, 101, widget.NewLabel("replacement")), mobile.FilterClaim)
}

func TestGestoMenuSoloDesdeElBordeIzquierdoHaciaLaDerecha(t *testing.T) {
	for _, tc := range []struct {
		name         string
		start, end   float32
		detail, open bool
		want         int
	}{
		{name: "left-to-right", start: 10, end: 90, want: 1},
		{name: "right-to-left", start: 10, end: -70},
		{name: "right-edge", start: 380, end: 300},
		{name: "middle", start: 100, end: 200},
		{name: "short", start: 10, end: 40},
		{name: "detail-back", start: 10, end: 100, detail: true},
		{name: "already-open", start: 10, end: 100, open: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			a := NuevoArmazon(widget.NewLabel("list"))
			a.Menu = NuevoLateral("menu", nil)
			bar := NuevaBarraMovil()
			bar.AlMenu = func() { calls++ }
			a.BarraMovil = bar
			a.Resize(fyne.NewSize(390, 700))
			a.DetalleAbierto, a.CajonAbierto = tc.detail, tc.open
			g := &gestosMoviles{}
			g.recibir(eventoGesto(mobile.FilterBegin, tc.start, 100, a))
			g.recibir(eventoGesto(mobile.FilterEnd, tc.end, 103, a))
			if calls != tc.want {
				t.Fatalf("menu action called %d times, want %d", calls, tc.want)
			}
		})
	}
}

func TestGestoTimeoutConservaPulsacionLargaYNoActuaTrasCapturar(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		l := listaDeGesto(t, &RefrescoLista{Arriba: func() bool { return true }, Actualizar: func() { t.Fatal("timed out gesture refreshed") }})
		g := &gestosMoviles{}
		comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 100, 100, l), mobile.FilterDefer)
		want := mobile.FilterPass
		if claimed {
			comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 100, 130), mobile.FilterClaim)
			want = mobile.FilterClaim
		}
		e := eventoGesto(mobile.FilterEnd, 100, 180, l)
		e.Time = e.Time.Add(3 * time.Second)
		comprobarFiltro(t, g, e, want)
	}
}

func TestGestoNoSeIniciaNiRefrescaSobreUnModal(t *testing.T) {
	l := listaDeGesto(t, &RefrescoLista{Arriba: func() bool { return true }, Actualizar: func() { t.Fatal("modal pull refreshed") }})
	w := test.NewWindow(l)
	t.Cleanup(w.Close)
	g := &gestosMoviles{canvas: w.Canvas()}
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 100, 100, l), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 100, 125), mobile.FilterClaim)
	w.Canvas().Overlays().Add(widget.NewLabel("modal"))
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 100, 180, l), mobile.FilterClaim)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 100, 100, l), mobile.FilterPass)
}
