// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestCapturasDeLoHechoAMedida deja en SYTUE_CAPTURAS un PNG por diálogo nuevo
// (clasificar, resumir, almacén propio en S3 y en NAS, calendario de la caducidad), a
// 1280×800 y a 360×740 y en los dos temas, para mirarlos contra la web. Sin la variable
// no hace nada.
func TestCapturasDeLoHechoAMedida(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	t.Cleanup(func() { tema.Elegir("") })
	guardar := func(p *Pantalla, nombre string) {
		t.Helper()
		img := p.win.Canvas().Capture()
		f, err := os.Create(filepath.Join(dir, nombre+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
	for _, claro := range []bool{true, false} {
		nombreTema := "claro"
		if !claro {
			nombreTema = "oscuro"
		}
		tema.Elegir(nombreTema)
		for _, tam := range []fyne.Size{fyne.NewSize(1280, 800), fyne.NewSize(360, 740)} {
			sufijo := fmt.Sprintf("%s-%d", nombreTema, int(tam.Width))
			nueva := func() *Pantalla {
				s := &servicioEtiquetas{
					etiquetas: `{"labels":[{"id":"l1","name":"Confidencial","block_external_share":true},{"id":"l2","name":"Uso interno"}]}`,
					contenido: "Acta de la junta de septiembre.",
				}
				srv := s.servidor(t)
				p := pantallaDePrueba(t, srv)
				p.IA = api.NewIA(srv.URL, sesionDoble{}, nil)
				p.win.SetContent(p.Objeto())
				p.win.Resize(tam)
				p.explor.Ventana = tam.Width
				return p
			}
			p := nueva()
			p.pedirClasificar(Elemento{ID: "f1", Nombre: "nóminas-septiembre.xlsx", Etiqueta: &api.DriveLabel{Name: "Uso interno"}})
			guardar(p, "clasificar-"+sufijo)

			p = nueva()
			p.pedirResumen(Elemento{ID: "f1", Nombre: "acta.md"})
			guardar(p, "resumen-"+sufijo)

			p = nueva()
			c := p.pedirCompartir(Elemento{ID: "f1", Nombre: "acta.odt"})
			c.caduca.PonerTexto("15/10/2026")
			ui.MostrarCalendario(c.caduca, c.caduca, "es", nil)
			guardar(p, "calendario-"+sufijo)

			for _, clase := range []string{"s3", "nas"} {
				p = nueva()
				p.lateral.Externo = &api.AlmacenExterno{Available: true}
				de := p.pedirExterno()
				de.elegirServidor(clase)
				if clase == "nas" {
					de.vista = "nas"
					de.nas = &api.EstadoNAS{Command: "docker run -d --network host fosrl/newt --id x --secret y", Online: true}
				}
				de.pintar()
				guardar(p, "externo-"+clase+"-"+sufijo)
			}
		}
	}
}

// TestCapturaDelLateralConLoNuevo deja en SYTUE_CAPTURAS el lateral de Drive a 1280 con
// «Traer mis datos» (un origen de Google conectado), el medidor y «Añadir
// almacenamiento» en «Tus nubes», en los dos temas. Sin la variable no hace nada.
func TestCapturaDelLateralConLoNuevo(t *testing.T) {
	dir := os.Getenv("SYTUE_CAPTURAS")
	if dir == "" {
		t.Skip("sin SYTUE_CAPTURAS")
	}
	t.Cleanup(func() { tema.Elegir("") })
	pasarela := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/instances":
			_, _ = io.WriteString(w, `{"instances":[{"id":"g","name":"Google","template_key":"google","status":"active"}]}`)
		default:
			_, _ = io.WriteString(w, `{"migrations":[]}`)
		}
	}))
	t.Cleanup(pasarela.Close)
	for _, nombreTema := range []string{"claro", "oscuro"} {
		tema.Elegir(nombreTema)
		p := pantallaDePrueba(t, (&servicioExterno{admin: true}).servidor(t))
		p.ConIntegraciones(api.NewIntegraciones(pasarela.URL, sesionDoble{}, nil))
		p.cargarExterno()
		p.cargarCuota()
		p.win.SetContent(p.Objeto())
		p.win.Resize(fyne.NewSize(1280, 800))
		p.lateral.Importar.Cargar()
		// La carga va en su propia rutina y su `fyne.Do` espera en la cola del driver de
		// pruebas hasta el siguiente punto de sincronía, que es una captura.
		for i := 0; i < 100 && p.lateral.Importar.Objeto(ui.AnchoUtilMenu, tema.Claro) == nil; i++ {
			time.Sleep(20 * time.Millisecond)
			p.win.Canvas().Capture()
		}
		p.Repintar()
		img := p.win.Canvas().Capture()
		f, err := os.Create(filepath.Join(dir, "lateral-"+nombreTema+"-1280.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
}
