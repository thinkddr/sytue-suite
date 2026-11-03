// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
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
	if got := g.filtrar(e); got != want {
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
			begin := g.filtrar(eventoGesto(mobile.FilterBegin, 100, 200, path...))
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
			g.filtrar(eventoGesto(mobile.FilterEnd, 100, 180, path...))
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
		menuInDetail bool
		want         int
	}{
		{name: "left-to-right", start: 10, end: 90, want: 1},
		{name: "right-to-left", start: 10, end: -70},
		{name: "right-edge", start: 380, end: 300},
		{name: "middle", start: 100, end: 200},
		{name: "short", start: 10, end: 40},
		{name: "detail-back", start: 10, end: 100, detail: true},
		{name: "conversation-menu", start: 10, end: 100, detail: true, menuInDetail: true, want: 1},
		{name: "conversation-middle", start: 100, end: 200, detail: true, menuInDetail: true},
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
			a.MenuEnDetalle = tc.menuInDetail
			g := &gestosMoviles{}
			g.filtrar(eventoGesto(mobile.FilterBegin, tc.start, 100, a))
			g.filtrar(eventoGesto(mobile.FilterEnd, tc.end, 103, a))
			if calls != tc.want {
				t.Fatalf("menu action called %d times, want %d", calls, tc.want)
			}
		})
	}
}

func TestGestoMenuEnDetalleSinBarraConservaConversacionYScroll(t *testing.T) {
	sinAnimacionDelMenu(t)
	a := NuevoArmazon(widget.NewLabel("conversación"))
	a.Menu = NuevoLateral("Canales", nil)
	a.DetalleAbierto, a.MenuEnDetalle, a.SinBarraMovil = true, true, true
	a.Resize(fyne.NewSize(390, 700))
	test.WidgetRenderer(a)
	a.Refresh()
	g := &gestosMoviles{}

	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 8, 200, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 80, 202, a), mobile.FilterClaim)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 100, 202, a), mobile.FilterClaim)
	if !a.CajonAbierto || !a.Menu.Visible() || !a.DetalleAbierto {
		t.Fatal("el gesto debe abrir los canales sin abandonar la conversación")
	}

	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 240, 250, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 170, 252, a), mobile.FilterClaim)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 160, 252, a), mobile.FilterClaim)
	if a.CajonAbierto || a.Menu.Visible() || !a.DetalleAbierto {
		t.Fatal("cerrar los canales debe volver a la misma conversación")
	}

	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 8, 200, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 80, 202, a), mobile.FilterClaim)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterCancel, 80, 202, a), mobile.FilterPass)
	if a.CajonAbierto || a.Menu.Visible() || !a.DetalleAbierto {
		t.Fatal("cancelar el gesto no debe dejar abierto el cajón ni cambiar de canal")
	}

	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 8, 200, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 80, 202, a), mobile.FilterClaim)
	a.MenuEnDetalle = false // una página secundaria tapa la conversación durante el gesto
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 100, 202, a), mobile.FilterClaim)
	if a.CajonAbierto || a.Menu.Visible() || !a.DetalleAbierto {
		t.Fatal("el gesto no debe abrir el cajón detrás de la página que sustituyó la conversación")
	}
	a.MenuEnDetalle = true

	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 8, 200, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 10, 300, a), mobile.FilterPass)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 8, 200, a, widget.NewEntry()), mobile.FilterPass)
	if a.CajonAbierto {
		t.Fatal("el desplazamiento vertical y la edición no deben abrir el cajón")
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

func TestGestoAperturaYCierreSiguenElDedoEnVivo(t *testing.T) {
	sinAnimacionDelMenu(t)
	a := NuevoArmazon(widget.NewLabel("lista"))
	a.Menu = NuevoLateral("Menú", nil)
	a.Resize(fyne.NewSize(390, 700))
	r := test.WidgetRenderer(a).(*armazonRenderer)
	a.Refresh()
	g := &gestosMoviles{}

	// Arrastre corto desde el borde izquierdo: muestra el cajón en vivo y cancela al soltar.
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 10, 200, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 110, 204, a), mobile.FilterClaim)
	if !a.arrastrandoCajon || !a.Menu.Visible() || a.Menu.Position().X != 100-a.Menu.Size().Width || !r.velo.Visible() {
		t.Fatalf("el cajón no sigue al dedo al abrir: arrastrando=%v pos=%v velo=%v", a.arrastrandoCajon, a.Menu.Position(), r.velo.Visible())
	}
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 40, 204, a), mobile.FilterClaim)
	if a.CajonAbierto || a.arrastrandoCajon || a.Menu.Visible() {
		t.Fatalf("un gesto corto dejó el cajón abierto: abierto=%v visible=%v", a.CajonAbierto, a.Menu.Visible())
	}

	// Arrastre completo desde el borde izquierdo: abre y asienta en x=0.
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 8, 200, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 60, 202, a), mobile.FilterClaim)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 96, 202, a), mobile.FilterClaim)
	if !a.CajonAbierto || a.arrastrandoCajon || a.Menu.Position().X != 0 {
		t.Fatalf("el gesto de apertura no asentó el cajón: abierto=%v pos=%v", a.CajonAbierto, a.Menu.Position())
	}

	// Desplazamiento vertical con el cajón abierto: pasa al scroll del menú sin reclamarlo.
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 120, 300, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 122, 220, a), mobile.FilterPass)
	if !a.CajonAbierto || a.arrastrandoCajon {
		t.Fatal("el scroll vertical interfirió con el cajón abierto")
	}

	// Arrastre corto hacia la izquierda: sigue al dedo y vuelve a abrirse al soltar antes del umbral.
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 220, 250, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 180, 252, a), mobile.FilterClaim)
	if !a.arrastrandoCajon || a.Menu.Position().X != -40 {
		t.Fatalf("el cajón no sigue al dedo al recoger: arrastrando=%v pos=%v", a.arrastrandoCajon, a.Menu.Position())
	}
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 195, 252, a), mobile.FilterClaim)
	if !a.CajonAbierto || a.Menu.Position().X != 0 {
		t.Fatalf("un cierre corto cerró el cajón: abierto=%v pos=%v", a.CajonAbierto, a.Menu.Position())
	}

	// Arrastre decidido hacia la izquierda: cierra el cajón y sincroniza AlCajon.
	cierres := 0
	a.AlCajon = func(abierto bool) {
		if !abierto {
			cierres++
		}
	}
	comprobarFiltro(t, g, eventoGesto(mobile.FilterBegin, 240, 250, a), mobile.FilterDefer)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterMove, 180, 252, a), mobile.FilterClaim)
	comprobarFiltro(t, g, eventoGesto(mobile.FilterEnd, 160, 252, a), mobile.FilterClaim)
	if a.CajonAbierto || a.Menu.Visible() || cierres != 1 {
		t.Fatalf("el gesto hacia la izquierda no cerró el cajón: abierto=%v visible=%v cierres=%d", a.CajonAbierto, a.Menu.Visible(), cierres)
	}
}

func TestBuscadorBarraLimpiarYAlturaMovil(t *testing.T) {
	t.Cleanup(TactilDePrueba(true))
	b := barraPruebaA(t, 568)
	montado := b.Montar(contenidoDePrueba())
	w := test.NewTempWindow(t, montado)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(568, 220))

	if b.buscador.limpiar.Visible() || b.buscador.campo.MargenDerecho != 0 {
		t.Fatal("el botón de vaciar aparece con el buscador vacío")
	}
	b.buscador.campo.PonerTexto("presupuesto")
	b.buscador.verAtajo()
	if !b.buscador.limpiar.Visible() || b.buscador.campo.MargenDerecho <= 0 {
		t.Fatal("escribir en el buscador no enseña el botón de vaciar ni reserva su margen")
	}

	filas := make([]fyne.CanvasObject, 0, 8)
	for range 8 {
		filas = append(filas, nuevaFilaResultado("Documento", "Fragmento", nil))
	}
	b.buscador.panel.Poner(filas)
	b.buscador.panel.Show()
	b.buscador.colocarPanel()
	b.buscador.panel.Refresh()
	if fin := b.buscador.panel.Position().Y + b.buscador.panel.Size().Height; fin > 220 {
		t.Fatalf("el panel del buscador desborda la pantalla apaisada: fin=%g > 220", fin)
	}
	if b.buscador.panel.r == nil || b.buscador.panel.r.desplaza == nil {
		t.Fatal("el panel acotado en altura no activó el contenedor desplazable")
	}

	test.Tap(b.buscador.limpiar)
	if b.buscador.campo.Texto() != "" || b.buscador.limpiar.Visible() || b.buscador.campo.MargenDerecho != 0 || b.buscador.panel.Visible() {
		t.Fatal("el botón de vaciar no limpió el texto ni cerró el panel")
	}
}

func TestGestoActualizarYMenuFuncionanBajoCapasHermanasEIndicador(t *testing.T) {
	sinAnimacionDelMenu(t)
	previous := esMovil
	esMovil = func() bool { return true }
	t.Cleanup(func() { esMovil = previous })
	t.Cleanup(TactilDePrueba(true))

	refrescos := 0
	desplazar := container.NewVScroll(cuerpoDe(900))
	lista := RefrescarAlTirar(desplazar, &RefrescoLista{
		Arriba:     func() bool { return desplazar.Offset.Y <= 0.5 },
		Actualizar: func() { refrescos++ },
	})
	armazon := NuevoArmazon(lista)
	armazon.Menu = NuevoLateral("Menú", widget.NewLabel("carpetas"))
	fabPulsado := 0
	fab := NuevoBotonFlotante("Nuevo", iconos.Mas, func() { fabPulsado++ })
	avisos := NuevaPilaAvisos()

	b := barraPruebaA(t, 390)
	montado := b.Montar(container.NewStack(armazon, container.New(CapaFlotante{}, fab), avisos))
	w := test.NewTempWindow(t, montado)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))

	raiz, ok := w.Canvas().Content().(*raizGestos)
	if !ok {
		t.Fatal("la ventana móvil no montó raizGestos")
	}
	raiz.gestos.canvas = w.Canvas()
	rend := test.WidgetRenderer(raiz).(*rendererGestos)

	// Simula `touchPath` de Fyne: el último hermano visible a pantalla completa
	// (`avisos`) eclipsa `armazon` y `lista` en `e.Path`.
	rutaEclipsada := []fyne.CanvasObject{raiz, avisos}

	// 1. Tirar hacia abajo sobre la lista actualiza y enseña el disco indicador mientras dura.
	comprobarFiltro(t, &raiz.gestos, eventoGesto(mobile.FilterBegin, 195, 220, rutaEclipsada...), mobile.FilterDefer)
	comprobarFiltro(t, &raiz.gestos, eventoGesto(mobile.FilterMove, 195, 310, rutaEclipsada...), mobile.FilterClaim)
	if !rend.disco.Visible() {
		t.Fatal("el indicador visual de refresco no se enseña durante el tirón")
	}
	comprobarFiltro(t, &raiz.gestos, eventoGesto(mobile.FilterEnd, 210, 320, rutaEclipsada...), mobile.FilterClaim)
	if refrescos != 1 {
		t.Fatalf("el tirón bajo capas hermanas refrescó %d veces; quiero 1", refrescos)
	}
	if rend.disco.Visible() {
		t.Fatal("el indicador visual sigue visible tras soltar")
	}

	// 2. Tirar empezando sobre el FAB no reclama el gesto como refresco.
	posFab := fyne.CurrentApp().Driver().AbsolutePositionForObject(fab)
	centroFab := fyne.NewPos(posFab.X+fab.Size().Width/2, posFab.Y+fab.Size().Height/2)
	comprobarFiltro(t, &raiz.gestos, eventoGesto(mobile.FilterBegin, centroFab.X, centroFab.Y, rutaEclipsada...), mobile.FilterPass)

	// 3. Deslizar desde el borde izquierdo abre el cajón aunque `avisos` eclipse `e.Path`.
	comprobarFiltro(t, &raiz.gestos, eventoGesto(mobile.FilterBegin, 10, 300, rutaEclipsada...), mobile.FilterDefer)
	comprobarFiltro(t, &raiz.gestos, eventoGesto(mobile.FilterMove, 80, 302, rutaEclipsada...), mobile.FilterClaim)
	comprobarFiltro(t, &raiz.gestos, eventoGesto(mobile.FilterEnd, 110, 302, rutaEclipsada...), mobile.FilterClaim)
	if !armazon.CajonAbierto {
		t.Fatal("el gesto de borde izquierdo no abrió el cajón bajo capas hermanas")
	}
}
