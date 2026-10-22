// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// El gesto sobre una plantilla tiene que bajar también la cabecera y los recientes;
// al llegar abajo debe poderse abrir el último documento sin otro desplazamiento.
func TestLaPortadaDesplazaPlantillasYRecientesJuntos(t *testing.T) {
	for _, tipo := range []Tipo{Documento, Hoja, Presentacion} {
		t.Run(string(tipo.Kind()), func(t *testing.T) {
			s := &servicio{lista: `{"documents":[{"id":"final","kind":"document","title":"Final"}],"next_cursor":""}`}
			srv := httptest.NewServer(s)
			t.Cleanup(srv.Close)
			p := NuevaPortada(tipo, nil, "es", api.NewDocs(srv.URL, sesionFija{}, srv.Client()))
			p.Lanzar = func(f func()) { f() }
			p.Estado = Listo
			for i := range 30 {
				p.Docs = append(p.Docs, api.Document{ID: strconv.Itoa(i), Kind: tipo.Kind(), Title: "Documento " + strconv.Itoa(i)})
			}
			for i := range 8 {
				p.Plantillas = append(p.Plantillas, api.DocTemplate{Key: strconv.Itoa(i), Kind: tipo.Kind(), Name: "Plantilla"})
			}
			p.PlantillasOrg = p.Plantillas
			p.Repintar()
			ventana := test.NewTempWindow(t, p.Objeto())
			ventana.SetPadded(false)
			abs := fyne.CurrentApp().Driver().AbsolutePositionForObject
			for paso, tam := range []fyne.Size{fyne.NewSize(1280, 640), fyne.NewSize(360, 640), fyne.NewSize(820, 360), fyne.NewSize(1280, 640)} {
				ventana.Resize(tam)
				_ = ventana.Canvas().Capture()
				w := p.vista
				w.scroll.ScrollToTop()
				fila := w.galeria.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*ui.DesplazaLado)
				ultimo := w.cuerpo.(*fyne.Container).Objects[len(p.Docs)-1]
				piezas := []fyne.CanvasObject{w.r.titulo, w.acceso, ultimo}
				antes := make([]float32, len(piezas))
				for i, pieza := range piezas {
					antes[i] = abs(pieza).Y
				}
				punto := abs(fila).Add(fyne.NewPos(20, 10))
				test.Scroll(ventana.Canvas(), punto, 0, -80)
				seMuevenJuntos := func() bool {
					for i, pieza := range piezas {
						if abs32(antes[i]-abs(pieza).Y-80) > 0.1 {
							return false
						}
					}
					return true
				}
				if !seMuevenJuntos() {
					t.Fatalf("a %v el gesto sobre plantillas no desplaza toda la portada: offset=%v", tam, w.scroll.Offset)
				}
				if paso == 0 {
					// Control negativo: congelar la cabecera reproduce el scroll aislado
					// anterior y debe romper la misma comprobación, sin fallar el CI.
					pos := w.r.titulo.Position()
					w.r.titulo.Move(pos.Add(fyne.NewPos(0, 80)))
					if seMuevenJuntos() {
						t.Fatal("el guardián no detecta una cabecera que queda fija")
					}
					w.r.titulo.Move(pos)
				}
				vertical := w.scroll.Offset.Y
				test.Scroll(ventana.Canvas(), abs(fila).Add(fyne.NewPos(20, 10)), -60, 0)
				if fila.X() <= 0 || w.scroll.Offset.Y != vertical {
					t.Errorf("a %v el gesto lateral no conserva su eje: galería=%v página=%v", tam, fila.X(), w.scroll.Offset.Y)
				}
				w.scroll.ScrollToBottom()
				pos := abs(ultimo)
				if pos.Y < 0 || pos.Y+ultimo.Size().Height > tam.Height+0.1 || pos.X < 0 || pos.X+ultimo.Size().Width > tam.Width+0.1 {
					t.Errorf("a %v no se alcanza entero el último reciente: %v/%v", tam, pos, ultimo.Size())
				}
				offset := w.scroll.Offset.Y
				desplazador := w.scroll
				p.Docs = append(p.Docs, api.Document{ID: "nuevo-" + strconv.Itoa(paso), Kind: tipo.Kind(), Title: "Nuevo"})
				p.Repintar()
				if w.scroll != desplazador || w.scroll.Offset.Y != offset {
					t.Errorf("al añadir recientes a %v la portada salta de %v a %v", tam, offset, w.scroll.Offset.Y)
				}
			}
			// La página siguiente se pide al alcanzar el final del desplazamiento
			// exterior y se añade sin sustituirlo ni perder su posición.
			desplazador := p.vista.scroll
			desplazador.ScrollToTop()
			p.Cursor = "siguiente"
			n := len(p.Docs)
			desplazador.ScrollToBottom()
			if len(p.Docs) != n+1 || p.Cursor != "" || p.vista.scroll != desplazador {
				t.Errorf("el final de la portada no pagina sobre el mismo desplazamiento: documentos=%d cursor=%q", len(p.Docs), p.Cursor)
			}
		})
	}
}
