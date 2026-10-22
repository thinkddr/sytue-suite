// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Notepads usa la navegación del armazón para heredar la continuidad del marco,
// conservando el cajón, el regreso al árbol y el título propio de los días.
func TestLaNavegacionMovilConservaElCajonYElRegresoAlArbol(t *testing.T) {
	p := NuevaPantalla(trInterno(t), "es", nil, nil, nil)
	p.activo = DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza"})
	p.activo.Vista = VistaArbol
	pulsar := func(rotulo string) {
		t.Helper()
		a := p.armazon(p.variante())
		b, ok := a.BarraMovil.(*ui.BarraMovil)
		if !ok {
			t.Fatal("Notepads no usa la barra móvil compartida")
		}
		b.Resize(fyne.NewSize(390, ui.AltoBarraMovil))
		var accion fyne.Tappable
		recorrerInterno(b, func(o fyne.CanvasObject) {
			if accesible, ok := o.(fyne.Accessible); ok && accesible.AccessibilityLabel() == rotulo {
				if boton, ok := o.(fyne.Tappable); ok {
					accion = boton
				}
			}
		})
		if accion == nil {
			t.Fatalf("la navegación no ofrece %q", rotulo)
		}
		test.Tap(accion)
	}
	pulsar(p.tr.T("ui.openNav", nil))
	if !p.cajon {
		t.Fatal("el botón del menú no abre el cajón")
	}
	pulsar(p.tr.T("ui.closeNav", nil))
	if p.cajon {
		t.Fatal("el botón del menú no cierra el cajón")
	}
	p.lienzo.Seleccionada = "n1"
	pulsar(p.tr.T("ui.backToList", nil))
	if p.lienzo.Seleccionada != "" || p.cajon {
		t.Fatal("volver al árbol no deja la lista visible con el cajón cerrado")
	}
	p.activo.Crudo.DailyDate = "2026-10-16"
	if got := p.barraMovil(false, false).Titulo; got != p.tr.T("app.title", nil) {
		t.Errorf("un día repite el título del cuaderno en la navegación: %q", got)
	}
}

func TestLasDisposicionesDeNotesSiguenLosCortesDeLaWeb(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/notes/src/components/NotebookSettings.tsx")
	if !strings.Contains(web, "grid grid-cols-2 gap-2 sm:grid-cols-3") {
		t.Fatal("la web ya no declara dos columnas móviles y tres desde sm")
	}
	ventana := float32(320)
	r := rejillaAjustes{ventana: func() float32 { return ventana }}
	for _, caso := range []struct {
		ancho    float32
		columnas int
	}{{320, 2}, {375, 2}, {390, 2}, {639, 2}, {640, 3}, {1024, 3}, {320, 2}} {
		ventana = caso.ancho
		if r.rejilla().Fijas != caso.columnas {
			t.Errorf("a %v de VENTANA el selector conserva otro número de columnas", ventana)
		}
	}
}

func TestLosAjustesDeNotesCabenEnMovilYAlRedimensionar(t *testing.T) {
	for _, ancho := range []float32{320, 375, 390} {
		t.Run(fmt.Sprint(ancho), func(t *testing.T) {
			p, w := pantallaModalMovil(t, ancho)
			p.dialogoAjustes(&p.cuadernos[0], "")
			d := p.ajustes
			t.Cleanup(d.Cerrar)
			titulo := d.Foco.(*ui.Entrada)
			titulo.PonerTexto("Cuaderno conservado al elegir")
			for _, medida := range []fyne.Size{{Width: ancho, Height: 568}, {Width: 1024, Height: 800}, {Width: ancho, Height: 260}, {Width: ancho, Height: 568}} {
				w.Resize(medida)
				d.Recolocar()
				partes := d.Cuerpo.(*fyne.Container).Objects[0].(*fyne.Container).Objects
				paleta := partes[1].(*fyne.Container).Objects[1].(*fyne.Container)
				grid := partes[2].(*fyne.Container).Objects[1].(*fyne.Container)
				if len(paleta.Objects) != 12 || len(grid.Objects) != 4 {
					t.Fatal("la regresión ha perdido colores o disposiciones")
				}
				comprobarHijosDelModal(t, paleta)
				comprobarHijosDelModal(t, grid)
				for _, o := range grid.Objects {
					opcion := o.(*Opcion)
					if opcion.Size().Height+0.01 < opcion.AltoPara(opcion.Size().Width) {
						t.Error("una disposición no reserva el alto de su ayuda")
					}
					recorrerInterno(opcion.Contenido, func(o fyne.CanvasObject) {
						if par, ok := o.(*ui.Parrafo); ok {
							for _, linea := range ui.LineasDe(par, par.Size().Width) {
								if ui.Medir(linea, par.Escalon, par.Peso).Width > par.Size().Width+0.1 {
									t.Errorf("la disposición corta %q a %v", linea, medida)
								}
							}
						}
					})
				}
				if grid.Position().Y+grid.Size().Height > partes[2].Size().Height+0.01 {
					t.Fatal("la sección siguiente pisa las disposiciones")
				}
				if medida.Width == ancho {
					d.Desplazador().ScrollToTop()
					capturar(t, w, fmt.Sprintf("ajustes-movil-%gx%g-arriba", ancho, medida.Height))
				}
				comprobarPieNotes(t, w, d)
				if medida.Width == ancho {
					capturar(t, w, fmt.Sprintf("ajustes-movil-%gx%g-abajo", ancho, medida.Height))
				}
			}
			partes := d.Cuerpo.(*fyne.Container).Objects[0].(*fyne.Container).Objects
			paleta := partes[1].(*fyne.Container).Objects[1].(*fyne.Container)
			paleta.Objects[11].(*Muestra).Tapped(nil)
			if titulo.Texto() != "Cuaderno conservado al elegir" || d.Foco != titulo {
				t.Fatal("elegir el último color pierde lo escrito o reconstruye el campo enfocado")
			}
		})
	}
}

func TestElRecordatorioDeNotesNoRecortaCamposNiAcciones(t *testing.T) {
	for _, ancho := range []float32{320, 375, 390} {
		t.Run(fmt.Sprint(ancho), func(t *testing.T) {
			p, w := pantallaModalMovil(t, ancho)
			p.recordatorios = map[string]api.NoteReminder{"n1": {NoteID: "n1", DueAt: time.Date(2026, 10, 15, 9, 0, 0, 0, time.Local)}}
			p.dialogoRecordatorio(Nota{ID: "n1", Tipo: "todo"})
			var d *ui.Dialogo
			recorrerInterno(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
				if x, ok := o.(*ui.Dialogo); ok {
					d = x
				}
			})
			if d == nil {
				t.Fatal("el recordatorio no se ha abierto")
			}
			t.Cleanup(d.Cerrar)
			for _, alto := range []float32{568, 260, 568} {
				w.Resize(fyne.NewSize(ancho, alto))
				d.Recolocar()
				col := d.Cuerpo.(*fyne.Container)
				fecha := col.Objects[0].(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*fyne.Container)
				comprobarHijosDelModal(t, fecha)
				comprobarPieNotes(t, w, d)
				capturar(t, w, fmt.Sprintf("recordatorio-movil-%gx%g", ancho, alto))
			}
			col := d.Cuerpo.(*fyne.Container)
			todoElDia := col.Objects[1].(*fyne.Container).Objects[0].(*ui.Conmutador)
			todoElDia.Tapped(nil)
			fecha := col.Objects[0].(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*fyne.Container)
			if len(fecha.Objects) != 1 || fecha.Objects[0].Size().Width != fecha.Size().Width {
				t.Fatal("todo el día sigue reservando el hueco de una hora oculta")
			}
		})
	}
}

func pantallaModalMovil(t *testing.T, ancho float32) (*Pantalla, fyne.Window) {
	t.Helper()
	w := test.NewTempWindow(t, container.NewWithoutLayout())
	w.SetPadded(false)
	w.Resize(fyne.NewSize(ancho, 568))
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	w.SetContent(p.Objeto())
	p.Yo = func() string { return "ada" }
	p.cuadernos = []Cuaderno{DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza", OwnerSub: "ada", Color: ColoresCuaderno[0]})}
	p.activo = p.cuadernos[0]
	if dir := os.Getenv("SYTUE_CAPTURAS"); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	return p, w
}

func comprobarHijosDelModal(t *testing.T, fila *fyne.Container) {
	t.Helper()
	for i, a := range fila.Objects {
		if !a.Visible() {
			continue
		}
		p, s := a.Position(), a.Size()
		if p.X < -0.01 || p.Y < -0.01 || p.X+s.Width > fila.Size().Width+0.01 || p.Y+s.Height > fila.Size().Height+0.01 {
			t.Errorf("el hijo %d sale de su sección: %v/%v en %v", i, p, s, fila.Size())
		}
		for _, b := range fila.Objects[i+1:] {
			q, z := b.Position(), b.Size()
			if b.Visible() && min(p.X+s.Width, q.X+z.Width)-max(p.X, q.X) > 0.1 && min(p.Y+s.Height, q.Y+z.Height)-max(p.Y, q.Y) > 0.1 {
				t.Fatal("dos controles se solapan dentro de la sección")
			}
		}
	}
}

func comprobarPieNotes(t *testing.T, w fyne.Window, d *ui.Dialogo) {
	t.Helper()
	pie := d.Pie.(*fyne.Container)
	comprobarHijosDelModal(t, pie)
	for _, o := range pie.Objects {
		if _, ok := o.(*ui.Boton); !ok {
			t.Fatal("el pie vuelve a agrupar acciones que no pueden partir")
		}
	}
	d.Desplazador().ScrollToBottom()
	ultimo := pie.Objects[len(pie.Objects)-1]
	driver := fyne.CurrentApp().Driver()
	p, q := driver.AbsolutePositionForObject(d), driver.AbsolutePositionForObject(ultimo)
	if q.Y < p.Y || q.Y+ultimo.Size().Height > p.Y+d.Size().Height+0.01 || q.X < p.X || q.X+ultimo.Size().Width > p.X+d.Size().Width+0.01 {
		t.Fatalf("Guardar no se alcanza en %v: acción=%v/%v, panel=%v/%v", w.Canvas().Size(), q, ultimo.Size(), p, d.Size())
	}
}
