// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

func TestLaRotacionDelLateralAlcanzaElUltimoDestinoConGestoYTeclado(t *testing.T) {
	defer TactilDePrueba(true)()
	t.Cleanup(tema.FormaDeLosGuardianes)
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		filas := make([]fyne.CanvasObject, 20)
		pulsados, enfocados := 0, 0
		for i := range filas {
			filas[i] = NuevaFilaLateral(fmt.Sprint(i), nil, func() { pulsados++ })
		}
		ultimo := filas[len(filas)-1].(*FilaLateral)
		ultimo.AlEnfocar = func() { enfocados++ }
		menu := NuevoLateral("Destinos", container.New(Columna{}, filas...)).ConCabecera(NuevoBoton("Crear", nil))
		menu.Pie = NuevoBoton("Cuenta", nil)
		w := test.NewTempWindow(t, menu)
		w.SetPadded(false)
		for _, tam := range []fyne.Size{{Width: 260, Height: 700}, {Width: 260, Height: 240}, {Width: 260, Height: 320}, {Width: 260, Height: 700}} {
			w.Resize(tam)
			r := test.WidgetRenderer(menu).(*lateralRenderer)
			s := r.desplaza
			w.Canvas().Unfocus()
			s.ScrollToTop()
			s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -10000)})
			controlEnViewportLateral(t, ultimo, s)
			test.Tap(ultimo)
			if pulsados == 0 {
				t.Fatal("el gesto alcanza el destino pero no permite activarlo")
			}
			w.Canvas().Unfocus()
			s.ScrollToTop()
			antes := enfocados
			w.Canvas().Focus(ultimo)
			controlEnViewportLateral(t, ultimo, s)
			if enfocados != antes+1 {
				t.Fatal("instalar el revelador pierde o duplica AlEnfocar")
			}
			menu.Refresh()
			controlEnViewportLateral(t, ultimo, s)
			controlEnViewportLateral(t, menu.Cabecera, menu)
			controlEnViewportLateral(t, menu.Pie, menu)
			if ultimo.Size().Height < 44 {
				t.Fatalf("el lateral encoge los blancos táctiles: forma %s, ventana %v, fila %v", forma, tam, ultimo.Size())
			}
		}
	}
}

func TestLaRotacionConservaElSueloTactilDeTodasLasFilasDelLateral(t *testing.T) {
	t.Cleanup(tema.FormaDeLosGuardianes)
	for _, forma := range []string{"marco", "carril"} {
		for _, tactil := range []bool{false, true} {
			for _, caso := range []struct {
				nombre    string
				configura func(*FilaLateral)
				alto      float32
			}{
				{"destino", func(*FilaLateral) {}, 38},
				{"compacta", func(f *FilaLateral) { f.Compacta = true }, 30},
				{"plegada", func(f *FilaLateral) { f.Plegada = true }, 38},
				{"rama", func(f *FilaLateral) { f.Rama = &RamaLateral{} }, 22},
			} {
				t.Run(fmt.Sprintf("%s/tactil=%t/%s", forma, tactil, caso.nombre), func(t *testing.T) {
					t.Cleanup(TactilDePrueba(tactil))
					tema.ElegirForma(forma)
					fila := NuevaFilaLateral("Destino", nil, nil)
					caso.configura(fila)
					w := test.NewTempWindow(t, container.New(Columna{}, fila))
					w.SetPadded(false)
					w.Resize(fyne.NewSize(260, 120))
					quiero := caso.alto
					if tactil {
						quiero = 44
					}
					if minimo, colocado := fila.MinSize().Height, fila.Size().Height; minimo != quiero || colocado != quiero {
						t.Fatalf("fila de %v px mínimos y %v px colocados; debe medir %v px", minimo, colocado, quiero)
					}
				})
			}
		}
	}
}

func TestLaRotacionDelLateralReutilizaElScrollYNoPersigueFocoDeOtraCapa(t *testing.T) {
	defer TactilDePrueba(true)()
	objetos := make([]fyne.CanvasObject, 15)
	for i := range objetos {
		objetos[i] = NuevoBoton(fmt.Sprint(i), nil)
	}
	s := container.NewVScroll(container.New(Columna{}, objetos...))
	menu := NuevoLateral("Destinos", s)
	fuera := NuevoBoton("Otra capa", nil)
	raiz := container.NewWithoutLayout(menu, fuera)
	w := test.NewTempWindow(t, raiz)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(300, 700))
	menu.Resize(fyne.NewSize(260, 160))
	fuera.Move(fyne.NewPos(30, 300))
	fuera.Resize(fyne.NewSize(120, 44))
	r := test.WidgetRenderer(menu).(*lateralRenderer)
	if r.desplaza != nil {
		t.Fatal("el lateral envolvió por segunda vez el Scroll de la app")
	}
	w.Canvas().Focus(fuera)
	menu.Refresh()
	if s.Offset.Y != 0 {
		t.Fatal("redimensionar persigue un control externo que coincide en geometría")
	}
	ultimo := objetos[len(objetos)-1].(*Boton)
	w.Canvas().Focus(ultimo)
	controlEnViewportLateral(t, ultimo, s)
	menu.Resize(fyne.NewSize(260, 120))
	controlEnViewportLateral(t, ultimo, s)
}

func TestLaRotacionNoAcumulaReveladoresDeScrollsRetirados(t *testing.T) {
	defer TactilDePrueba(true)()
	filas := make([]fyne.CanvasObject, 15)
	for i := range filas {
		filas[i] = NuevoBotonIcono(iconos.Menu, fmt.Sprint(i), nil)
	}
	cuerpo := container.New(Columna{}, filas...)
	viejo := container.NewVScroll(cuerpo)
	w := test.NewTempWindow(t, viejo)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(260, 180))
	RevelarFocoEnScroll(viejo, cuerpo)
	nuevo := container.NewVScroll(cuerpo)
	w.SetContent(nuevo)
	RevelarFocoEnScroll(nuevo, cuerpo)
	ultimo := filas[len(filas)-1].(*BotonIcono)
	w.Canvas().Focus(ultimo)
	if viejo.Offset.Y != 0 || nuevo.Offset.Y == 0 {
		t.Fatal("el foco conserva el revelador de un Scroll retirado")
	}
	controlEnViewportLateral(t, ultimo, nuevo)
}

func controlEnViewportLateral(t *testing.T, control, viewport fyne.CanvasObject) {
	t.Helper()
	d := fyne.CurrentApp().Driver()
	p, v := d.AbsolutePositionForObject(control), d.AbsolutePositionForObject(viewport)
	if p.X < v.X-1 || p.Y < v.Y-1 || p.X+control.Size().Width > v.X+viewport.Size().Width+1 || p.Y+control.Size().Height > v.Y+viewport.Size().Height+1 {
		t.Fatalf("%T fuera del viewport: %v/%v en %v/%v", control, p, control.Size(), v, viewport.Size())
	}
}

// El driver de pruebas no admite CanvasForObject sin ventanas. Preparar el menú
// antes de abrirlas debe conservar los callbacks sin consultar todavía un lienzo.
func TestLaRotacionPreparaElLateralAntesDeAbrirUnaVentana(t *testing.T) {
	anterior := fyne.CurrentApp()
	app := test.NewApp()
	t.Cleanup(func() { fyne.SetCurrentApp(anterior) })
	app.Settings().SetTheme(tema.Sytue{})
	// La app aún no expone ventanas; el driver de Fyne conserva su lienzo ficticio
	// para los Refresh de las primitivas al preparar el renderer. Sin la guardia de
	// lienzoDe, CanvasForObject devolvería ese lienzo aunque no haya ventana real.
	fyne.SetCurrentApp(appSinVentanasLateral{App: app, driver: driverSinVentanasLateral{Driver: app.Driver()}})
	if len(fyne.CurrentApp().Driver().AllWindows()) != 0 {
		t.Fatal("el fixture debe empezar sin ventanas")
	}
	boton := NuevoBoton("Destino", nil)
	menu := NuevoLateral("Destinos", container.New(Columna{}, boton))
	test.WidgetRenderer(menu)
	menu.Resize(fyne.NewSize(260, 240))
	if c := lienzoDe(menu); c != nil {
		t.Fatal("el menú sin montar aparenta tener un lienzo")
	}
	fyne.SetCurrentApp(app)
	w := test.NewTempWindow(t, menu)
	w.Resize(fyne.NewSize(260, 240))
	w.Canvas().Focus(boton)
	if w.Canvas().Focused() != fyne.Focusable(boton) {
		t.Fatal("preparar antes de abrir la ventana perdió el foco del destino")
	}
}

type appSinVentanasLateral struct {
	fyne.App
	driver fyne.Driver
}

func (a appSinVentanasLateral) Driver() fyne.Driver { return a.driver }

type driverSinVentanasLateral struct{ fyne.Driver }

func (driverSinVentanasLateral) AllWindows() []fyne.Window { return nil }

func TestLaRotacionDelLateralConservaElOrdenDeTabEntreCabeceraDestinosYCuenta(t *testing.T) {
	defer TactilDePrueba(true)()
	cabecera := NuevoBoton("Crear", func() {})
	primero := NuevaFilaLateral("Primero", nil, func() {})
	segundo := NuevaFilaLateral("Segundo", nil, func() {})
	cuenta := NuevoBoton("Cuenta", func() {})
	menu := NuevoLateral("Destinos", container.New(Columna{}, primero, segundo)).ConCabecera(cabecera)
	menu.Pie = cuenta
	w := test.NewTempWindow(t, menu)
	w.SetPadded(false)
	for _, alto := range []float32{700, 320, 700} {
		w.Resize(fyne.NewSize(260, alto))
		w.Canvas().Focus(cabecera)
		for _, destino := range []fyne.Focusable{primero, segundo, cuenta} {
			w.Canvas().FocusNext()
			if w.Canvas().Focused() != destino {
				t.Fatalf("alto %g: Tab pasa a %T; quería %T", alto, w.Canvas().Focused(), destino)
			}
		}
	}
}
