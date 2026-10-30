// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestLaRotacionOfficeCompactaConservaUnaFilaYTodosLosComandos(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	ctx := &Contexto{Tipo: Hoja, Estado: Listo, Zoom: 100}
	r := Genericas()
	r.Anadir("datos", bandaAncha)
	r.Anadir("formulas", bandaAncha)
	var ultimo *BotonCinta
	for _, p := range r.Visibles(Hoja) {
		r.Sustituir(p.ID, func(c *Contexto) fyne.CanvasObject {
			objetos := objetosDeBanda(bandaAncha(c))
			ultimo = NuevoBotonCinta(iconos.EdNegrita, "Último comando", func() {})
			return Grupos(append(objetos, NuevoGrupo("Último", ultimo))...)
		})
	}
	m := NuevoMarco(ctx, r, nil)
	w := test.NewTempWindow(t, m)
	w.SetPadded(false)
	for _, tam := range []fyne.Size{{Width: 568, Height: 272}, {Width: 756, Height: 299}, {Width: 756, Height: 369}, {Width: 390, Height: 763}, {Width: 1024, Height: 748}, {Width: 568, Height: 272}} {
		w.Resize(tam)
		compacto := tam.Width > tam.Height && tam.Height < 400
		if m.compacto != compacto || m.cabecera.Visible() == compacto || m.estado.Visible() == compacto {
			t.Fatalf("%v: cambió la disposición fuera de su corte", tam)
		}
		if !compacto {
			continue
		}
		c := m.cinta
		if c.Size().Height != 53 || c.tira.Visible() || c.mas.Visible() || !c.banda.Visible() {
			t.Fatal("la cinta compacta reserva otra fila o pierde sus comandos")
		}
		test.Tap(c.selector)
		menu := c.menuPestanas
		fin := menu.Elementos[len(menu.Elementos)-1].(*ui.ElementoMenu)
		for range len(menu.Elementos) - 1 {
			w.Canvas().Focused().TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
		}
		if w.Canvas().Focused() != fyne.Focusable(fin) {
			t.Fatal("la navegación no alcanza la última pestaña")
		}
		comprobarControlEn(t, w, fin, menu)
		if fin.Size().Height < 44 {
			t.Fatal("las pestañas del menú comprimen su blanco táctil")
		}
		test.Tap(fin)
		if c.activa != len(r.Visibles(Hoja))-1 {
			t.Fatal("no se alcanza la última pestaña")
		}
		if test.WidgetRenderer(c.selector).(*chipRenderer).texto.Text != c.activaTira().Rotulo {
			t.Fatal("el selector conserva el nombre de la pestaña anterior")
		}
		c.banda.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(-10000, 0)})
		comprobarControlEn(t, w, ultimo, c.banda)
		if ultimo.Size().Width < 44 || ultimo.Size().Height < 44 {
			t.Fatal("la banda comprime sus blancos táctiles")
		}
		w.Canvas().Focus(ultimo)
		comprobarControlEn(t, w, ultimo, c.banda)
		test.Tap(ultimo)
		if c.Size().Height != 53 || c.desplegada != nil {
			t.Fatal("usar el comando expandió la cinta")
		}
	}
}

type lienzoDeComandosCompacto struct {
	*lienzoFijo
	comandos []Comando
}

func (l *lienzoDeComandosCompacto) Mandar(c Comando, _ string) { l.comandos = append(l.comandos, c) }

func (*lienzoDeComandosCompacto) Formato() Formato {
	return Formato{PuedeDeshacer: true, PuedeRehacer: true}
}

func TestLaRotacionOfficeConservaArchivoRenombrarCompartirZoomYVolver(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	texto := ui.NuevaEntrada("")
	texto.PonerTexto("Documento conservado")
	lienzo := &lienzoDeComandosCompacto{lienzoFijo: &lienzoFijo{texto}}
	compartidos, vueltas, exportados := 0, 0, 0
	ctx := &Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, Titulo: "Propuesta", Lienzo: lienzo}
	ctx.AlRenombrar = func(string) {}
	ctx.Compartir = func(fyne.CanvasObject) { compartidos++ }
	ctx.Exportar = func(string) { exportados++ }
	m := NuevoMarco(ctx, Genericas(), func() { vueltas++ })
	w := test.NewTempWindow(t, m)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(568, 272))
	pulsar := func(rotulo string) {
		t.Helper()
		test.Tap(m.cinta.archivo)
		menu := m.cinta.archivo.menu
		for range len(menu.Elementos) {
			it, ok := w.Canvas().Focused().(*ui.ElementoMenu)
			if !ok {
				t.Fatal("el menú no enfoca una acción")
			}
			if it.Texto == rotulo {
				comprobarControlEn(t, w, it, menu)
				if it.Inerte || it.Size().Height < 44 {
					t.Fatalf("Archivo no ofrece %s con área táctil", rotulo)
				}
				test.Tap(it)
				return
			}
			it.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDown})
		}
		t.Fatalf("Archivo perdió %q", rotulo)
	}
	for _, rotulo := range []string{"header.share", "status.zoom.in", "action.undo", "action.redo", "file.downloadPdf", "file.rename"} {
		pulsar(rotulo)
	}
	if compartidos != 1 || exportados != 1 || ctx.Zoom != 110 || texto.Texto() != "Documento conservado" || len(lienzo.comandos) != 2 {
		t.Fatal("las acciones compactas perdieron sus callbacks o el contenido")
	}
	if m.renombrar == nil {
		t.Fatal("renombrar enfocó el campo oculto de la cabecera")
	}
	campo := m.renombrar.Cuerpo.(*ui.Entrada)
	campo.PonerTexto("Renombrado")
	if ctx.Titulo != "Renombrado" {
		t.Fatal("el cambio de nombre no recorre el callback de Cabecera")
	}
	w.Resize(fyne.NewSize(390, 763))
	if m.compacto || m.cabecera.titulo.Texto() != "Renombrado" || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("volver a vertical perdió el título o conservó un menú del modo horizontal")
	}
	w.Resize(fyne.NewSize(568, 272))
	pulsar("ui.backToList")
	if vueltas != 1 {
		t.Fatal("Archivo perdió la vuelta a la portada")
	}
}

func TestLaRotacionOfficeUsaLaVentanaParaOrientacionYElAreaSeguraParaAltura(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	for _, caso := range []struct {
		ventana, util fyne.Size
		compacto      bool
	}{
		{fyne.NewSize(390, 844), fyne.NewSize(390, 280), false}, // teclado en vertical
		{fyne.NewSize(844, 390), fyne.NewSize(756, 299), true},
		{fyne.NewSize(844, 420), fyne.NewSize(756, 299), true}, // límites de ventana mayores que el área útil
		{fyne.NewSize(1024, 768), fyne.NewSize(1024, 700), false},
	} {
		if officeCompacto(caso.ventana, caso.util) != caso.compacto {
			t.Fatalf("ventana=%v, útil=%v: modo compacto incorrecto", caso.ventana, caso.util)
		}
	}
}

func TestLaRotacionOfficeNoDejaFocoEnLasFilasQueDesaparecen(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	m := NuevoMarco(&Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, AlRenombrar: func(string) {}, Exportar: func(string) {}}, Genericas(), func() {})
	w := test.NewTempWindow(t, m)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 763))
	for _, f := range []fyne.Focusable{m.cabecera.titulo, m.cabecera.volver, m.estado.mas, m.estado.deslizador, m.cinta.tiras[0]} {
		w.Canvas().Focus(f)
		if w.Canvas().Focused() != f {
			t.Fatalf("el fixture no ha enfocado %T antes de girar", f)
		}
		w.Resize(fyne.NewSize(568, 272))
		if w.Canvas().Focused() != fyne.Focusable(m.cinta.archivo) {
			t.Fatalf("el foco sigue en %T, cuya fila desapareció", w.Canvas().Focused())
		}
		w.Resize(fyne.NewSize(390, 763))
	}
	test.Tap(m.cabecera.titulo)
	if !m.cabecera.titulo.Enfocado() {
		t.Fatal("el toque no enfoca el editor real del título")
	}
	w.Resize(fyne.NewSize(568, 272))
	if w.Canvas().Focused() != fyne.Focusable(m.cinta.archivo) {
		t.Fatal("el editor interno del título conserva el foco estando oculto")
	}
	w.Canvas().Focus(m.cinta.selector)
	test.Tap(m.cinta.selector)
	w.Resize(fyne.NewSize(390, 763))
	if w.Canvas().Focused() != fyne.Focusable(m.cinta.archivo) || len(w.Canvas().Overlays().List()) != 0 {
		t.Fatal("el selector oculto conserva el foco o su menú")
	}
	// Descargar está oculto por debajo de md: abrirlo desde una tableta real,
	// antes de reducir la ventana, evita pulsar un control invisible del fixture.
	w.Resize(fyne.NewSize(1024, 768))
	if !m.cabecera.descargar.Visible() {
		t.Fatal("el fixture de descargas necesita su botón visible")
	}
	test.Tap(m.cabecera.descargar)
	menu := m.cabecera.menu
	if menu == nil || !menu.Abierto() {
		t.Fatal("el fixture no ha abierto el menú de descargas antes de girar")
	}
	w.Resize(fyne.NewSize(568, 272))
	if menu.Abierto() || len(w.Canvas().Overlays().List()) != 0 || w.Canvas().Focused() != fyne.Focusable(m.cinta.archivo) {
		t.Fatalf("al girar: compacto=%v, menú=%v, capas=%d, foco=%T; esperaba Archivo", m.compacto, menu.Abierto(), len(w.Canvas().Overlays().List()), w.Canvas().Focused())
	}
}

// El modo táctil no cambia el dispositivo del driver de pruebas. Este adaptador
// conserva su ventana y canvas, pero hace que la política compartida vea un móvil.
type appMovilDeOffice struct{ fyne.App }

func (a appMovilDeOffice) Driver() fyne.Driver { return driverMovilDeOffice{a.App.Driver()} }

type driverMovilDeOffice struct{ fyne.Driver }

func (d driverMovilDeOffice) Device() fyne.Device { return dispositivoMovilDeOffice{d.Driver.Device()} }

type dispositivoMovilDeOffice struct{ fyne.Device }

func (dispositivoMovilDeOffice) IsMobile() bool { return true }

func TestLaRotacionOfficeRetiraElFocoDeEscrituraEnDispositivoMovil(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	anterior := fyne.CurrentApp()
	fyne.SetCurrentApp(appMovilDeOffice{anterior})
	t.Cleanup(func() { fyne.SetCurrentApp(anterior) })
	m := NuevoMarco(&Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, AlRenombrar: func(string) {}}, Genericas(), nil)
	w := test.NewTempWindow(t, m)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 763))
	test.Tap(m.cabecera.titulo)
	if !ui.EsMovil() || !m.cabecera.titulo.Enfocado() {
		t.Fatal("el fixture no ha enfocado el editor del título en un dispositivo móvil")
	}
	w.Resize(fyne.NewSize(568, 272))
	if w.Canvas().Focused() != nil || m.cabecera.titulo.Enfocado() {
		t.Fatal("la fila retirada conserva el teclado o lo abre al enfocar Archivo")
	}
	w.Canvas().Focus(m.cinta.selector)
	w.Resize(fyne.NewSize(390, 763))
	if w.Canvas().Focused() != nil {
		t.Fatal("volver a vertical mantiene el teclado del selector retirado")
	}
}

// El menú abierto tiene su propio foco; al cerrarlo antes de retirar la cabecera
// ya no queda el botón original que permitía detectar qué fila iba a desaparecer.
func TestLaRotacionOfficeConDescargasRespetaLaPoliticaDeFoco(t *testing.T) {
	for _, caso := range []struct {
		nombre        string
		tactil, movil bool
		ancho         float32
	}{
		{"tactil", true, false, 568},
		{"movil", true, true, 568},
		{"escritorio", false, false, 1024},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			defer ui.TactilDePrueba(caso.tactil)()
			anterior := fyne.CurrentApp()
			if caso.movil {
				fyne.SetCurrentApp(appMovilDeOffice{anterior})
				defer fyne.SetCurrentApp(anterior)
			}
			m := NuevoMarco(&Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, Exportar: func(string) {}}, Genericas(), nil)
			w := test.NewTempWindow(t, m)
			w.SetPadded(false)
			w.Resize(fyne.NewSize(1024, 768))
			test.Tap(m.cabecera.descargar)
			menu := m.cabecera.menu
			if menu == nil || !menu.Abierto() || len(w.Canvas().Overlays().List()) != 1 {
				t.Fatal("el fixture no ha abierto las descargas de la cabecera visible")
			}
			focoMenu := w.Canvas().Focused()
			w.Resize(fyne.NewSize(caso.ancho, 272))
			if !caso.tactil {
				if m.compacto || !menu.Abierto() || len(w.Canvas().Overlays().List()) != 1 || w.Canvas().Focused() != focoMenu {
					t.Fatal("reducir la altura en escritorio cerró o robó el foco de las descargas visibles")
				}
				return
			}
			if !m.compacto || menu.Abierto() || len(w.Canvas().Overlays().List()) != 0 {
				t.Fatal("la cabecera retirada conserva el menú o una capa")
			}
			var esperado fyne.Focusable = m.cinta.archivo
			if caso.movil {
				esperado = nil
			}
			if w.Canvas().Focused() != esperado {
				t.Fatalf("foco=%T; quiero %T sin abrir el teclado móvil", w.Canvas().Focused(), esperado)
			}
		})
	}
}
