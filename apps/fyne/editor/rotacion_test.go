// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func TestRotacionDelEditorConservaElDocumentoYLasHerramientas(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	tr, err := i18n.New(i18n.Default, i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	for _, tipo := range []Tipo{Documento, Hoja, Presentacion} {
		t.Run(fmt.Sprint(tipo), func(t *testing.T) {
			texto := ui.NuevaEntrada("")
			texto.PonerTexto("Documento que debe sobrevivir a la rotación")
			ctx := &Contexto{Tipo: tipo, Estado: Listo, Zoom: 125, Lienzo: &lienzoFijo{texto}}
			m := NuevoMarco(ctx, Genericas(), nil)
			b := ui.NuevaBarraSuite(tr, nil, iconos.Docs).ConHojaASangre()
			t.Cleanup(b.Parar)
			w := test.NewTempWindow(t, b.Montar(m))
			w.SetPadded(false)
			for _, tam := range []fyne.Size{{Width: 320, Height: 700}, {Width: 568, Height: 320}, {Width: 844, Height: 390}, {Width: 1024, Height: 768}, {Width: 768, Height: 1024}, {Width: 320, Height: 700}} {
				w.Resize(tam)
				if m.centro != fyne.CanvasObject(texto) || texto.Texto() != "Documento que debe sobrevivir a la rotación" || ctx.Zoom != 125 {
					t.Fatal("girar reconstruyó el documento o perdió la edición/zoom")
				}
				if !m.centro.Visible() || m.centro.Size().Width <= 0 || m.centro.Size().Height <= 0 {
					t.Fatalf("%v: el documento no tiene área visible: %v", tam, m.centro.Size())
				}
				ultimo := m.carril.botones[len(m.carril.botones)-1]
				m.carril.desplaza.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -10000)})
				comprobarControlEn(t, w, ultimo, m.carril)
				if ultimo.Size().Width < 44 || ultimo.Size().Height < 44 {
					t.Fatal("el carril comprime los blancos táctiles")
				}
				m.carril.desplaza.ScrollToTop()
				w.Canvas().Focus(ultimo)
				comprobarControlEn(t, w, ultimo, m.carril)
				test.Tap(ultimo)
				if ctx.Panel != PanelIA || m.panel == nil || !m.panel.Visible() {
					t.Fatal("el último panel no se puede abrir después de girar")
				}
				comprobarControlEn(t, w, m.panel.cerrar, m)
				guardarRotacion(t, w, fmt.Sprintf("native-rotacion-%d-%gx%g.png", tipo, tam.Width, tam.Height))
				test.Tap(m.panel.cerrar)
				if ctx.Panel != SinPanel || !m.centro.Visible() || m.centro != fyne.CanvasObject(texto) {
					t.Fatal("cerrar el panel no devuelve el documento")
				}
			}
		})
	}
}

func comprobarControlEn(t *testing.T, w fyne.Window, control, area fyne.CanvasObject) {
	t.Helper()
	d := fyne.CurrentApp().Driver()
	p, a := d.AbsolutePositionForObject(control), d.AbsolutePositionForObject(area)
	s := control.Size()
	if p.X < a.X-0.5 || p.Y < a.Y-0.5 || p.X+s.Width > a.X+area.Size().Width+0.5 || p.Y+s.Height > a.Y+area.Size().Height+0.5 || p.X+s.Width > w.Canvas().Size().Width+0.5 || p.Y+s.Height > w.Canvas().Size().Height+0.5 {
		t.Fatalf("control fuera del área visible: %v/%v en %v/%v", p, s, a, area.Size())
	}
}

func TestLosPanelesDelEditorSeRepartenAlGirarSinPerderSuEstado(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	entrada := ui.NuevaEntrada("")
	entrada.PonerTexto("consulta conservada")
	ctx := &Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, Panel: PanelBuscar, Ajustes: true, Paneles: func(Panel) fyne.CanvasObject { return entrada }}
	m := NuevoMarco(ctx, Genericas(), nil)
	w := test.NewTempWindow(t, m)
	w.SetPadded(false)
	for _, tam := range []fyne.Size{{Width: 1024, Height: 768}, {Width: 844, Height: 320}, {Width: 320, Height: 700}, {Width: 1024, Height: 768}} {
		w.Resize(tam)
		if ctx.Panel != PanelBuscar || !ctx.Ajustes || entrada.Texto() != "consulta conservada" {
			t.Fatal("girar cambió el estado de los paneles")
		}
		comprobarControlEn(t, w, m.ajustes.cerrar, m)
		if tam.Width == 1024 {
			if !m.centro.Visible() || !m.panel.Visible() || m.centro.Size().Width < ui.AnchoContenidoMinimo {
				t.Fatal("la tablet no recupera las columnas y el lienzo")
			}
		} else if m.centro.Visible() || m.panel.Visible() || m.ajustes.Size().Width != tam.Width-anchoCarril() {
			t.Fatal("los paneles compactos se solapan o dejan el lienzo sin espacio")
		}
	}
}

func TestElPanelDelEditorDesplazaSuUltimaAccionConPocaAltura(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	ultimo := ui.NuevoBoton("Última acción", nil)
	objetos := make([]fyne.CanvasObject, 0, 10)
	for range 9 {
		objetos = append(objetos, ui.NuevaEntrada("Campo"))
	}
	objetos = append(objetos, ultimo)
	p := NuevoPanelLateral(&Contexto{}, "Panel", container.New(ui.Columna{Hueco: 4}, objetos...), false)
	w := test.NewTempWindow(t, p)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(288, 120))
	if p.desplaza == nil {
		t.Fatal("el formulario no se puede desplazar")
	}
	p.desplaza.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -10000)})
	comprobarControlEn(t, w, ultimo, p.desplaza)
}

func TestRotacionDelPanelConListaYFormularioMantieneAmbosDesplazamientos(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	ultimo := ui.NuevoBoton("Enviar comentario", nil)
	entrada := ui.NuevaEntrada("Comentario")
	entrada.PonerTexto("borrador que se conserva")
	filas := make([]fyne.CanvasObject, 20)
	for i := range filas {
		filas[i] = ui.NuevoBoton(fmt.Sprint(i), nil)
	}
	lista := container.NewVScroll(container.New(ui.Columna{}, filas...))
	pie := container.New(ui.Columna{Hueco: 4}, ui.NuevaEntrada("Asunto"), entrada, ultimo)
	cuerpo := container.NewBorder(nil, pie, nil, nil, lista)
	ctx := &Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, Panel: PanelComentarios, Paneles: func(Panel) fyne.CanvasObject { return cuerpo }}
	m := NuevoMarco(ctx, Genericas(), nil)
	w := test.NewTempWindow(t, m)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(568, 320))
	p := m.panel
	p.desplaza.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -10000)})
	comprobarControlEn(t, w, ultimo, p.desplaza)
	anterior := p.desplaza.Offset
	lista.ScrollToBottom()
	if lista.Offset.Y <= 0 || anterior.Y <= 0 {
		t.Fatal("la lista y el formulario deben poder desplazarse por separado")
	}
	m.Recomponer()
	if m.panel.desplaza.Offset != anterior || lista.Offset.Y <= 0 || entrada.Texto() != "borrador que se conserva" {
		t.Fatal("recomponer perdió el scroll o el borrador del panel")
	}
	comprobarControlEn(t, w, ultimo, m.panel.desplaza)
	w.Resize(fyne.NewSize(1024, 768))
	w.Resize(fyne.NewSize(568, 320))
	m.panel.desplaza.ScrollToBottom()
	comprobarControlEn(t, w, ultimo, m.panel.desplaza)
	if entrada.Texto() != "borrador que se conserva" {
		t.Fatal("girar perdió el borrador")
	}
}

func guardarRotacion(t *testing.T, w fyne.Window, nombre string) {
	t.Helper()
	dir := os.Getenv("SYTUE_CINTA_CAPTURAS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, nombre))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}
