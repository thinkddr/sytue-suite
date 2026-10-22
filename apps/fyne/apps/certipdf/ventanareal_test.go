//go:build ventanareal

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func init() { runtime.LockOSThread() } // GLFW exige el hilo principal

// TestMain, con SYTUE_VENTANA_REAL, abre CertiPDF en una ventana DE VERDAD (GL de Mesa,
// no el pintor de software de los tests) contra un motor fingido, abre el panel «Firmas»,
// pulsa «Verificar las firmas» con el ratón (xdotool) y deja las capturas en esa carpeta.
// Va en TestMain y no en un test porque el bucle de la ventana tiene que correr en el
// hilo principal. Se lanza bajo Xvfb:
//
//	SYTUE_VENTANA_REAL=/tmp/vr xvfb-run -a -s '-screen 0 1280x800x24' \
//	  go test -tags ventanareal -run XXX ./apps/certipdf/
//
// No es un guardián: es la prueba de «si de paridad = ventana real» del panel de
// verificación. Sin la variable no hace nada y corren los tests de siempre.
func TestMain(m *testing.M) {
	if dir := os.Getenv("SYTUE_VENTANA_REAL"); dir != "" {
		if err := ventanaReal(dir); err != nil {
			fmt.Println("VENTANA REAL: FALLO:", err)
			os.Exit(1)
		}
		fmt.Println("VENTANA REAL: bien")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func ventanaReal(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		return err
	}
	m := &motorDeFirmas{informe: informeDePrueba}
	srv := httptest.NewServer(m)
	defer srv.Close()
	doc := &Documento{pdf: api.NewPDF(srv.URL, srv.URL, nil, nil), ruta: "/PDF/a.pdf", fichero: api.File{Name: "a.pdf"}, datos: []byte("%PDF-")}

	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})
	w := a.NewWindow("Sytue CertiPDF")
	w.SetPadded(false)
	p := NuevaPantalla(tr.T)
	p.Idioma = "es"
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	w.Show()

	captura := func(nombre string) error {
		if out, err := exec.Command("import", "-window", "root", dir+"/"+nombre+".png").CombinedOutput(); err != nil {
			return fmt.Errorf("import %s: %w %s", nombre, err, out)
		}
		return nil
	}
	fallo := make(chan error, 1)
	go func() {
		defer fyne.Do(a.Quit)
		pausa := func() { time.Sleep(2 * time.Second) }
		fyne.Do(func() { p.Abrir(doc) })
		pausa()
		fyne.Do(func() { p.PanelAbierto = true; p.panel.Abrir(PanelFirmas); p.Recomponer() })
		pausa()
		var x, y, ancho, alto float32
		fyne.DoAndWait(func() {
			var b *ui.Boton
			todos(p.panel.Objeto(), func(o fyne.CanvasObject) {
				if k, ok := o.(*ui.Boton); ok {
					b = k
				}
			})
			if b == nil {
				return
			}
			pos := absoluta(b)
			x, y, ancho, alto = pos.X, pos.Y, b.Size().Width, b.Size().Height
		})
		if ancho == 0 {
			fallo <- fmt.Errorf("no hay botón «Verificar las firmas»")
			return
		}
		fmt.Printf("BOTON x=%.2f y=%.2f ancho=%.2f alto=%.2f\n", x, y, ancho, alto)
		if err := captura("1-antes"); err != nil {
			fallo <- err
			return
		}
		cx, cy := fmt.Sprint(int(x+ancho/2)), fmt.Sprint(int(y+alto/2))
		if out, err := exec.Command("xdotool", "mousemove", cx, cy, "click", "1").CombinedOutput(); err != nil {
			fallo <- fmt.Errorf("xdotool: %w %s", err, out)
			return
		}
		pausa()
		fyne.DoAndWait(func() {
			n := 0
			todos(p.panel.Objeto(), func(o fyne.CanvasObject) {
				if r, ok := o.(*recuadro); ok {
					n++
					fmt.Printf("FICHA %d y=%.2f alto=%.2f ancho=%.2f\n", n, absoluta(r).Y, r.Size().Height, r.Size().Width)
				}
			})
		})
		if m.llamadas.Load() != 1 {
			fallo <- fmt.Errorf("el clic real hizo %d verificaciones; tenía que hacer una", m.llamadas.Load())
			return
		}
		fallo <- captura("2-informe")
	}()
	a.Run()
	select {
	case err := <-fallo:
		return err
	default:
		return fmt.Errorf("la ventana se cerró sin acabar el recorrido")
	}
}
