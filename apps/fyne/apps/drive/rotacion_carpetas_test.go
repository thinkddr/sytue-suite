// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La franja al 45 % dejaba solo 58,25/38,45 px visibles de una tarjeta de 102:
// pasaba el guardián de tamaño del widget pero ocultaba su nombre y «Carpeta».
func TestLaRotacionDeDriveConservaCarpetasLegiblesYAlcanzaLosArchivos(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	t.Cleanup(tema.FormaDeLosGuardianes)
	tema.ElegirForma("marco")
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 568
	e.Elementos = []Elemento{
		{ID: "c1", Nombre: "Actas", Carpeta: true, Tamano: -1},
		{ID: "f1", Nombre: "acta.md", Tamano: 10},
		{ID: "f2", Nombre: "b.pdf", Tamano: 2048},
	}
	marcadas, menus, abiertos := 0, 0, ""
	e.AlSeleccionar = func(string) { marcadas++ }
	e.AlMenu = func(Elemento, fyne.CanvasObject) { menus++ }
	e.AlAbrir = func(el Elemento) { abiertos = el.ID }
	vista := e.Objeto(tema.Claro)
	arm := ui.NuevoArmazon(vista)
	arm.Menu = ui.NuevoLateral("Drive", nil)
	barra, parar := ui.BarraSuiteDePrueba(e.tr, iconos.Drive)
	t.Cleanup(parar)
	w := test.NewTempWindow(t, barra.Montar(arm))
	w.SetPadded(false)
	for _, forma := range []string{"marco", "carril"} {
		tema.ElegirForma(forma)
		for _, tam := range []fyne.Size{{Width: 568, Height: 320}, {Width: 1024, Height: 768}, {Width: 568, Height: 320}} {
			w.Resize(tam)
			arm.Refresh()
			s := e.listaFranja
			if s == nil {
				t.Fatal("la franja táctil no tiene un viewport desplazable")
			}
			s.ScrollToTop()
			tarjeta := e.tarjetas[0].t
			r := tarjeta.r
			if r.nombre.Text != "Actas" || r.meta.Text != "Carpeta" {
				t.Fatal("la carpeta perdió su nombre o subtítulo")
			}
			for _, texto := range []fyne.CanvasObject{r.nombre, r.meta} {
				objetoDentroDeDrive(t, texto, tarjeta)
				if tam.Height == 320 {
					objetoDentroDeDrive(t, texto, s)
				}
			}
			for _, accion := range []fyne.CanvasObject{r.casilla, r.zonaMas} {
				objetoDentroDeDrive(t, accion, tarjeta)
				if accion.Size().Width < 44 || accion.Size().Height < 44 {
					t.Fatalf("acción táctil de %v; necesita 44×44", accion.Size())
				}
			}
			if r.casilla.Position().X+r.casilla.Size().Width > r.zonaMas.Position().X {
				t.Fatal("seleccionar y abrir el menú de la carpeta se solapan")
			}
			antesMarcar, antesMenu := marcadas, menus
			test.Tap(r.casilla)
			test.TapAt(tarjeta, r.zonaMas.Position().Add(fyne.NewPos(22, 22)))
			if marcadas != antesMarcar+1 || menus != antesMenu+1 {
				t.Fatal("los mandos de la carpeta no responden por separado")
			}
			if tam.Height != 320 {
				continue
			}
			capturarCarpetasDrive(t, w, forma, "carpeta")
			// El gesto desde el cuerpo llega hasta la lista; no basta con que su
			// viewport mida algo si la última fila sigue detrás del borde inferior.
			p := fyne.CurrentApp().Driver().AbsolutePositionForObject(tarjeta)
			test.Scroll(w.Canvas(), p.Add(fyne.NewPos(80, 30)), 0, -10000)
			if s.Offset.Y == 0 {
				t.Fatal("el desplazamiento sobre la carpeta no llega al viewport del cuerpo")
			}
			e.filas.ScrollToBottom()
			var ultimo *ui.FilaDrive
			recorrer(e.filas, func(o fyne.CanvasObject) {
				if f, ok := o.(*ui.FilaDrive); ok && f.Nombre == "b.pdf" {
					ultimo = f
				}
			})
			if ultimo == nil {
				t.Fatal("el desplazamiento no alcanza el archivo final")
			}
			objetoDentroDeDrive(t, ultimo, s)
			abiertos = ""
			test.TapAt(ultimo, fyne.NewPos(100, ultimo.Size().Height/2))
			if abiertos != "f2" {
				t.Fatal("el archivo alcanzado no se puede abrir")
			}
			if e.Refresco.Arriba() {
				t.Fatal("el final del cuerpo se confunde con el principio para actualizar")
			}
			capturarCarpetasDrive(t, w, forma, "archivos")
		}
	}
}

func TestLaRotacionDeDriveConMuchasCarpetasNoEncierraLosArchivosEnOtroScroll(t *testing.T) {
	defer ui.TactilDePrueba(true)()
	t.Cleanup(tema.FormaDeLosGuardianes)
	tema.ElegirForma("marco")
	e := NuevoExplorador(traductor(t))
	e.Fase, e.Ventana = Exito, 568
	for i := range 5 {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprint(i), Nombre: fmt.Sprintf("Carpeta %d", i), Carpeta: true, Tamano: -1})
	}
	for i := range 20 {
		e.Elementos = append(e.Elementos, Elemento{ID: fmt.Sprintf("f%d", i), Nombre: fmt.Sprintf("Archivo %d.txt", i)})
	}
	vista := e.Objeto(tema.Claro)
	w := test.NewTempWindow(t, vista)
	w.SetPadded(false)
	for _, alto := range []float32{216, 260, 216} {
		w.Resize(fyne.NewSize(520, alto)) // Cabecera de 95 y cuerpos de 121/165.
		s := e.listaFranja
		s.ScrollToTop()
		for _, tarjeta := range e.tarjetas {
			p, v := fyne.CurrentApp().Driver().AbsolutePositionForObject(tarjeta.t), fyne.CurrentApp().Driver().AbsolutePositionForObject(s)
			s.ScrollToOffset(fyne.NewPos(0, s.Offset.Y+p.Y-v.Y-16))
			objetoDentroDeDrive(t, tarjeta.t.r.nombre, s)
			objetoDentroDeDrive(t, tarjeta.t.r.meta, s)
		}
		anterior := s.Offset.Y
		lista := e.filas
		// Marcar o recargar repinta, pero no debe devolver el viewport a la primera
		// carpeta ni sustituir la lista virtualizada.
		w.SetContent(e.Objeto(tema.Claro))
		if e.listaFranja != s || e.filas != lista || s.Offset.Y != anterior {
			t.Fatal("repintar pierde la posición o la instancia de la lista")
		}
	}
}

func capturarCarpetasDrive(t *testing.T, w fyne.Window, forma, zona string) {
	t.Helper()
	dir := os.Getenv("SYTUE_ROTACION_CAPTURAS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SYTUE_CAPTURA", filepath.Join(dir, fmt.Sprintf("native-rotacion-drive-%s-%s-568x320.png", zona, forma)))
	guardar(t, w.Canvas().Capture())
}

func objetoDentroDeDrive(t *testing.T, objeto, viewport fyne.CanvasObject) {
	t.Helper()
	d := fyne.CurrentApp().Driver()
	p, v := d.AbsolutePositionForObject(objeto), d.AbsolutePositionForObject(viewport)
	if p.X < v.X-1 || p.Y < v.Y-1 || p.X+objeto.Size().Width > v.X+viewport.Size().Width+1 || p.Y+objeto.Size().Height > v.Y+viewport.Size().Height+1 {
		t.Fatalf("%T fuera de %T: %v/%v en %v/%v", objeto, viewport, p, objeto.Size(), v, viewport.Size())
	}
}
