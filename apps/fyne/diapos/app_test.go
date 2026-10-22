// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaAppEmpiezaEnLaPortadaYVuelve: sin documento, Diapos enseña la portada de
// presentaciones (no «Abriendo…» para siempre, que es lo que hacía); un contenido que se
// abre lleva al editor, se escribe, se guarda con el autoguardado y «Volver» regresa a
// la portada guardando lo que quedaba.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el guardado de «Volver», `guardado`
// sigue vacío al volver (lo guardaría el reloj 1,5 s después, o nadie si se cierra la
// ventana antes). Que el reloj quede desarmado lo guarda `editor.TestAhora…`.
func TestLaAppEmpiezaEnLaPortadaYVuelve(t *testing.T) {
	a := NuevaApp(traductor(t), "es", nil, nil)
	a.lanzar = func(f func()) { f() }
	w := test.NewWindow(a.Objeto()) // la cierra sinRelojDeCabecera: cerrarla dos veces revienta
	w.Resize(fyne.NewSize(1280, 800))
	a.Arrancar()
	if a.raiz.Objects[0] != a.portada.Objeto() {
		t.Fatal("sin documento, la portada")
	}
	guardado := make(chan string, 4)
	a.AbrirContenido("Mía", `{"slides":[{"id":"s","boxes":[]}]}`, true, func(m string) error {
		guardado <- m
		return nil
	})
	if a.Editor() == nil || a.Contexto().SoloLectura || a.Contexto().Estado != editor.Listo {
		t.Fatal("un contenido que se puede escribir abre el editor, listo")
	}
	a.Editor().NuevaForma("rect")
	w.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: mobile.KeyBack})
	// `lanzar` es síncrono aquí: el guardado tiene que haber salido ya, sin esperar al
	// reloj de 1,5 s, que además queda desarmado para no repetirlo.
	select {
	case m := <-guardado:
		if len(m) < 20 {
			t.Errorf("se guardó %q", m)
		}
	default:
		t.Fatal("«Volver» no guarda lo que quedaba")
	}
	if a.abierta != nil || a.raiz.Objects[0] != a.portada.Objeto() {
		t.Error("«Volver» enseña la portada")
	}
	// Una conversión de Drive no tiene dónde guardarse: solo lectura.
	a.AbrirContenido("Convertida", `{"slides":[{"id":"s","boxes":[]}]}`, false, nil)
	if !a.Contexto().SoloLectura {
		t.Error("lo convertido se enseña en solo lectura")
	}
	a.Fallo(EstadoDe(&api.Error{Status: 403}))
	if a.Contexto().Estado != editor.SinPermiso {
		t.Error("un 403 al abrir es «sin permiso»")
	}
	sinRelojDeCabecera(w)
}

// TestElMenuArchivoDeDiaposCreaYAbre: con servicio, «Nuevo» del menú Archivo es el
// alta de la portada; «Abrir…» lleva el `.pptx` y el `.odp` —lo que acepta la web para
// una presentación— al mismo `cargar` que «Abrir con», y el `.docx` a su app por el
// sistema. Y cambiar de presentación guarda antes lo que quedaba de la anterior.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin poner `ctx.Abrir`, el menú no lleva
// «Abrir…»; y sin `soltar` en `nueva`, abrir la segunda no guardaba la primera.
func TestElMenuArchivoDeDiaposCreaYAbre(t *testing.T) {
	a := NuevaApp(traductor(t), "es", api.NewDocs("http://127.0.0.1:1", nil, nil), nil)
	a.lanzar = func(f func()) { f() }
	var cargados, fuera []string
	a.AbrirArchivo = &editor.AbrirArchivo{
		Tipo: editor.Presentacion,
		Elegir: func(listo func([]string)) {
			listo([]string{"/tmp/Plan.pptx", "/tmp/Viejo.odp", "/tmp/Acta.docx"})
		},
		Cargar: func(_ context.Context, ap api.Apertura) error {
			cargados = append(cargados, ap.Local)
			return nil
		},
		Fuera:  func(r string) error { fuera = append(fuera, r); return nil },
		Lanzar: func(f func()) { f() },
	}
	w := test.NewWindow(a.Objeto()) // la cierra sinRelojDeCabecera
	w.Resize(fyne.NewSize(1280, 800))
	guardado := make(chan string, 4)
	a.AbrirContenido("Mía", `{"slides":[{"id":"s","boxes":[]}]}`, true, func(m string) error {
		guardado <- m
		return nil
	})
	ctx := a.Contexto()
	if ctx.Nuevo == nil || ctx.Abrir == nil {
		t.Fatal("el menú Archivo lleva «Nuevo» y «Abrir…»")
	}
	ctx.Abrir()
	if strings.Join(cargados, " ") != "file:///tmp/Plan.pptx file:///tmp/Viejo.odp" {
		t.Errorf("a Diapos le llegan %v", cargados)
	}
	if len(fuera) != 1 || fuera[0] != "/tmp/Acta.docx" {
		t.Errorf("el .docx lo abre Docus, por el sistema: %v", fuera)
	}
	a.Editor().NuevaForma("rect")
	a.AbrirContenido("Otra", `{"slides":[{"id":"s","boxes":[]}]}`, false, nil)
	select {
	case <-guardado:
	default:
		t.Error("abrir otra presentación no guarda antes lo que quedaba de la primera")
	}
	sinRelojDeCabecera(w)
}

// TestDiaposDescargaYVersionaConLaPiezaComun: abierta del servicio con permiso de
// escritura, «Descargar» pide la exportación a Docs y deja el fichero con el nombre de
// la web en la carpeta de descargas; «Guardar versión» y el historial van a su ruta.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin `archivo` en AbrirID, el menú no tenía
// dónde descargar (`ctx.Exportar` nil).
func TestDiaposDescargaYVersionaConLaPiezaComun(t *testing.T) {
	var rutas []string
	var pedidaVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rutas = append(rutas, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/state"):
			_, _ = w.Write([]byte(`{"seq":1,"state":{"lamport":1,"registers":{"doc:slides":` +
				`{"value":"{\"slides\":[{\"id\":\"s\",\"boxes\":[]}]}","dot":{"a":"x","c":1},"lamport":1}}}}`))
		case strings.HasSuffix(r.URL.Path, "/exports") && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"export":{"id":"e1"}}`))
		case strings.HasSuffix(r.URL.Path, "/exports/e1"):
			_, _ = w.Write([]byte("%PDF-1.7"))
		case strings.HasSuffix(r.URL.Path, "/versions") && r.Method == http.MethodPost:
			cuerpo, _ := io.ReadAll(r.Body)
			pedidaVersion = string(cuerpo)
			_, _ = w.Write([]byte(`{"version":{"id":"v1"}}`))
		case strings.HasSuffix(r.URL.Path, "/versions"):
			_, _ = w.Write([]byte(`{"versions":[{"id":"v1","label":"Antes"}]}`))
		default:
			_, _ = w.Write([]byte(`{"document":{"id":"d1","kind":"presentation","title":"Plan"},"role":"editor"}`))
		}
	}))
	defer srv.Close()
	a := NuevaApp(traductor(t), "es", api.NewDocs(srv.URL, nil, srv.Client()), api.NewSlides(srv.URL, nil, srv.Client()))
	a.lanzar = func(f func()) { f() }
	a.EnlaceDoc = func(id string) string { return "https://docs.test/edit?id=" + id }
	dir := t.TempDir()
	a.Carpeta = func() (string, error) { return dir, nil }
	test.NewTempWindow(t, a.Objeto()).Resize(fyne.NewSize(1280, 800))
	a.AbrirID("d1", "Plan")
	ctx := a.Contexto()
	if ctx.Exportar == nil || ctx.GuardarVersion == nil || ctx.Versiones == nil || ctx.RestaurarVersion == nil {
		t.Fatal("abierta con permiso, el menú descarga y versiona")
	}
	// «Compartir» también, como Docus y Calcus: sin él el botón estaba encendido y no
	// hacía nada (visto en Windows).
	if ctx.Compartir == nil {
		t.Error("abierta del servicio, «Compartir» no tiene qué hacer")
	}
	ctx.Exportar("pdf")
	if datos, err := os.ReadFile(filepath.Join(dir, "Plan.pdf")); err != nil || string(datos) != "%PDF-1.7" {
		t.Errorf("la descarga deja %q (%v)", datos, err)
	}
	if ctx.Exportando != "" {
		t.Error("acabada la descarga, el menú vuelve a estar encendido")
	}
	ctx.GuardarVersion("Antes de enviar")
	var vs []api.DocVersion
	ctx.Versiones(func(l []api.DocVersion) { vs = l })
	if len(vs) != 1 || vs[0].Label != "Antes" {
		t.Errorf("el historial trae %+v", vs)
	}
	if !slices.Contains(rutas, "POST /api/v1/documents/d1/versions") {
		t.Errorf("«Guardar versión» no ha llegado al servicio: %v", rutas)
	}
	if !strings.Contains(pedidaVersion, `"label":"Antes de enviar"`) {
		t.Errorf("«Guardar versión» no lleva la etiqueta escrita: %s", pedidaVersion)
	}
}

// sinRelojDeCabecera cierra la ventana y ejecuta lo que las rutinas de fondo dejaron en
// la cola de `fyne.Do`. El reloj del chip «Guardado hace…» (`editor.Cabecera`) ya no
// puede pisar a otra prueba: desde el fork v2.8.1-sytue.4 su `fyne.Do` lo ejecuta la
// rutina de la prueba que esté corriendo, en su siguiente punto de sincronía, así que no
// hace falta dormir los 5 s de su paso.
func sinRelojDeCabecera(w fyne.Window) {
	w.Close()
	fyne.DoAndWait(func() {})
}
