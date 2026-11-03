// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

func TestLasHerramientasDeFormatoConservanLaSeleccionYLaEscritura(t *testing.T) {
	for _, ancho := range []float32{390, 1280} {
		t.Run(fmt.Sprint(ancho), func(t *testing.T) {
			ed := NuevoEditorNota(trInterno(t), Leer("<p>hola mundo</p>"), "")
			terminadas := 0
			ed.AlTerminar = func() { terminadas++ }
			w := test.NewWindow(ed)
			t.Cleanup(w.Close)
			w.SetPadded(false)
			w.Resize(fyne.NewSize(ancho, 844))
			c := w.Canvas()
			c.Focus(ed.Zona())
			ed.ancla, ed.cur = Pos{0, 0}, Pos{0, 4}
			ed.repintar()
			comprobar := func() {
				t.Helper()
				if terminadas != 0 || c.Focused() != ed.Zona() || ed.Seleccionado() != "hola" {
					t.Fatalf("usar la barra termina la edición, pierde el foco o cambia la selección: %d, %T, %q",
						terminadas, c.Focused(), ed.Seleccionado())
				}
			}
			test.Tap(ed.botones["b"])
			comprobar()
			if ed.HTML() != "<p><b>hola</b> mundo</p>" {
				t.Fatalf("la negrita no se aplica sólo a la selección: %s", ed.HTML())
			}
			test.Tap(ed.botones["undo"])
			comprobar()
			if ed.HTML() != "<p>hola mundo</p>" {
				t.Fatalf("deshacer desde la barra deja %s", ed.HTML())
			}

			// Abrir una herramienta conserva el foco en su capa; aplicarla lo devuelve
			// al cuerpo con la misma selección, también con la barra envuelta en móvil.
			test.Tap(ed.botones["color"])
			ed.zona.comprobarSalida()
			if terminadas != 0 || len(c.Overlays().List()) == 0 || ed.Seleccionado() != "hola" {
				t.Fatal("abrir la paleta cierra la nota o pierde su selección")
			}
			var muestra *Muestra
			recorrerInterno(c.Overlays().Top(), func(o fyne.CanvasObject) {
				if m, ok := o.(*Muestra); ok && m.Rotulo == ColoresTexto[0] {
					muestra = m
				}
			})
			if muestra == nil {
				t.Fatal("la paleta no ofrece su primer color")
			}
			test.Tap(muestra)
			comprobar()
			if len(c.Overlays().List()) != 0 || !strings.Contains(ed.HTML(), "color:"+ColoresTexto[0]) {
				t.Fatalf("elegir un color no cierra la paleta y lo aplica: %s", ed.HTML())
			}
			ed.zona.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
			if terminadas != 1 {
				t.Fatal("Escape ya no termina explícitamente la edición")
			}
			ed.zona.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyReturn, Modifier: fyne.KeyModifierShortcutDefault})
			if terminadas != 2 {
				t.Fatal("Ctrl/⌘+Intro ya no termina explícitamente la edición")
			}
		})
	}
}

func TestLaBarraDelCuadernoNoCierraLaNotaAbierta(t *testing.T) {
	for _, ancho := range []float32{390, 1280} {
		for _, vista := range []string{"list", "columns"} {
			t.Run(fmt.Sprintf("%g-%s", ancho, vista), func(t *testing.T) {
				cuaderno, notas := cuadernoDePrueba(vista)
				p, _ := pantallaConServicio(t, nil, notas...)
				p.activo, p.cuadernos = cuaderno, []Cuaderno{cuaderno}
				w := test.NewWindow(nil)
				t.Cleanup(w.Close)
				w.SetPadded(false)
				w.Resize(fyne.NewSize(ancho, 844))
				p.win = w
				w.SetContent(p.Objeto())
				n, ok := p.nota("n2")
				if !ok {
					t.Fatal("falta la nota de texto del cuaderno de prueba")
				}
				p.abrirEditor(n)
				s := p.edicion
				if s == nil {
					t.Fatal("no se ha abierto la nota para escribir")
				}
				ed := s.editor
				ed.ancla, ed.cur = Pos{0, 0}, Pos{0, 6}
				ed.repintar()
				c := w.Canvas()
				comprobar := func() {
					t.Helper()
					// El driver de pruebas ejecuta fyne.Do en el acto. Esta es la misma
					// comprobación que el driver real encola hasta que se asienta el foco.
					ed.zona.comprobarSalida()
					if p.edicion != s || p.lienzo.Editor != ed || ed.Seleccionado() != "Veinte" || ed.HTML() != "<p>Veinte grandes.</p>" {
						t.Fatal("una herramienta del cuaderno cierra o reconstruye la nota que se escribe")
					}
				}
				var exportar *ui.BotonIcono
				recorrerInterno(w.Content(), func(o fyne.CanvasObject) {
					if b, ok := o.(*ui.BotonIcono); ok && b.Rotulo == p.tr.T("export.label", nil) {
						exportar = b
					}
				})
				if exportar == nil {
					t.Fatal("falta la herramienta de exportación del cuaderno")
				}
				c.Focus(exportar)
				comprobar()
				// Repintar sustituye los botones de la barra, pero el foco todavía puede
				// estar en el que abrió una herramienta. Eso tampoco termina la sesión.
				p.Repintar()
				comprobar()
				p.barra.AlAbrirFiltro()
				test.Tap(p.barra.campoFiltro()) // enfoca su editor interno, no sólo el wrapper
				comprobar()
				if !p.barra.campoFiltro().Enfocado() {
					t.Fatal("conservar la edición roba el foco al filtro")
				}
				c.Focus(ed.Zona())
				if p.capturaInerte {
					t.Fatal("el campo externo debe estar disponible para comprobar la salida")
				}
				c.Focus(p.campoCaptura)
				ed.zona.comprobarSalida()
				if p.edicion != nil {
					t.Fatal("salir a otro campo ajeno a la barra ya no termina la edición")
				}
			})
		}
	}
}
