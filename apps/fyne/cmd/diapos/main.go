// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command diapos es Sytue Diapos: el editor de presentaciones en Fyne.
//
// Es su propio binario y no una pantalla dentro de otro: en nativo cada producto de la
// suite es una aplicación (CONVENTIONS.md §13.6), con su identificador de paquete, su
// icono y su esquema de URL, que es como se llaman entre sí.
//
// Arranca en la portada de presentaciones (la de la web en `/diapos/`) y abre
// una presentación por su enlace (`sytue-slides:/edit?id=…`, el mismo documento que abre
// la web en `/diapos/edit?id=…`), que se lee y se guarda en el servicio; por un FICHERO del
// disco con el JSON de un registro `doc:slides`, que se guarda en ese mismo fichero; o
// convertida de Drive, en solo lectura. Lo que corre en cualquier sitio, sin CGO y sin
// servidor gráfico, es `go test ./diapos/...`.
package main

// La portada y la papelera son las de `editor` (portada.go, papelera.go), compartidas por
// los tres editores; lo que las hace de este producto es montarlas aquí con su tipo, como
// las páginas de su app en la web montan las vistas comunes de `@sytue/office`.
//
// paridad-web: apps/frontend/apps/diapos/src/pages/index.astro
// paridad-web: apps/frontend/apps/diapos/src/pages/trash.astro

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/diapos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(tema.Sytue{})

	// La sesión, la apariencia y el idioma son TRANSVERSALES a las trece apps: esto lo
	// lee de donde lo dejó la última que lo tocó. `Abrir` no habla con la red.
	s := sesion.Abrir(a, "")

	idioma := s.Idioma()
	tr, err := i18n.New(idioma, i18n.Common(), catalogos.Docs())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El título es el NOMBRE del producto, que es marca y no se traduce; sale de
	// FyneApp.toml, que es donde ya está declarado una vez para las cinco plataformas.
	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)

	// La portada de presentaciones y el editor hablan con el servicio con la sesión
	// TRANSVERSAL: sin sesión, la portada lo dice y ofrece entrar.
	d := diapos.NuevaApp(tr.T, idioma,
		api.NewDocs(s.URL("docs"), s.Sesion, nil), api.NewSlides(s.URL("slides"), s.Sesion, nil))
	d.Asistir = api.NewIA(s.URL("ai"), s.Sesion, nil).Asistir // el panel «Asistente»
	// Lo que copia «Compartir», como en Docus y Calcus: el editor en la web bajo la
	// organización activa (ADR 0056) y el enlace público sin ella.
	org := s.Sesion.Organizacion
	d.EnlaceDoc = func(id string) string { return api.AppURL(s.Apex(), "slides", org(), "edit?id="+url.QueryEscape(id)) }
	d.EnlacePublico = func(token string) string { return api.AppURL(s.Apex(), "slides", "", "s/"+url.PathEscape(token)) }
	// «Presentar con Cast»: programa el directo en Sytue Cast y abre su página de emisión,
	// bajo la organización activa como la web (`urlDeEmision`).
	d.Cast = &diapos.Cast{
		Cliente: api.NewCast(s.URL("cast"), s.Sesion, nil),
		Abrir: func(id string) {
			_ = api.AbrirFuera(a, api.AppURL(s.Apex(), "cast", org(), "studio?live="+url.QueryEscape(id)))
		},
	}
	d.Directorio = api.NewDirectorio(s.URL("id"), s.Sesion, nil)
	d.AlTitulo = func(t string) {
		if t == "" {
			t = a.Metadata().Name
		}
		w.SetTitle(t)
	}
	// La galería de plantillas de «Crear nuevo» y su selector de tema (editor/plantillas.go).
	d.Portada().ConPlantillas = true
	d.Portada().AlEntrar = func() {
		go func() {
			if err := s.Entrar(context.Background()); err == nil {
				fyne.Do(d.Arrancar)
			}
		}()
	}
	// Entrar o salir desde el menú de cuenta de ESTA o de otra app recarga la lista.
	dentro := s.Estado().Dentro()
	s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() != dentro {
			dentro = e.Dentro()
			d.Arrancar()
		}
	})
	// «Insertar → Imagen»: el selector de ficheros del SISTEMA (`ficheros.Elegir`), el
	// mismo con el que CertiPDF abre del disco; la web dice `accept="image/*"`.
	//
	// En el móvil `ficheros.Elegir` es el selector del sistema que pone la sesión
	// (`sesion/elegir_movil.go`), con lo elegido ya copiado a una ruta.
	d.ElegirImagen = func(listo func([]byte, bool)) {
		editor.ElegirDelSistema(func(rutas []string) {
			ext := strings.ToLower(filepath.Ext(rutas[0]))
			if !slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".webp"}, ext) {
				return
			}
			datos, err := os.ReadFile(rutas[0]) //nolint:gosec // G304: la ruta la ha elegido la persona en el diálogo del sistema
			if err != nil {
				return
			}
			listo(datos, ext == ".jpg" || ext == ".jpeg")
		})
	}
	// «Multimedia → Subir fichero…»: un audio o un vídeo pequeño, que se incrusta. Uno
	// grande no se lee: se dice que no cabe.
	d.ElegirMedio = func(listo func(string, []byte, bool)) {
		editor.ElegirDelSistema(func(rutas []string) {
			ruta, nombre := rutas[0], filepath.Base(rutas[0])
			if diapos.TipoMedio(nombre) == "" {
				return
			}
			if st, err := os.Stat(ruta); err == nil && st.Size() > diapos.MaxMedioIncrustado {
				listo(nombre, nil, true)
				return
			}
			datos, err := os.ReadFile(ruta) //nolint:gosec // G304: la ruta la ha elegido la persona en el diálogo del sistema
			if err != nil {
				return
			}
			listo(nombre, datos, false)
		})
	}
	w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Slides).ConSinSesionPropia().ConHojaASangre().Montar(d.Objeto()))
	w.Resize(fyne.NewSize(1280, 800))
	d.Arrancar()

	// El argumento es el JSON de un registro `doc:slides` del disco, que se guarda en ese
	// mismo fichero. Un `.pptx` o un `.odp` también llega como argumento, pero ese lo
	// convierte el servicio por `AceptarFicheros`, abajo; y en Linux y en Windows el
	// sistema arranca también así las aperturas por esquema, que atienden los receptores.
	if len(os.Args) > 1 && strings.EqualFold(filepath.Ext(os.Args[1]), ".json") {
		abrirJSON(d, os.Args[1])
	}
	// `sytue-slides:/edit?id=…` es el documento del servicio, el mismo que abre la web
	// en `/diapos/edit?id=…` (el Hub lo emite desde lo reciente y la búsqueda): se lee y se
	// guarda allí. Es ruta PROPIA: no es la página «no encontrada».
	s.RutaPropia(func(u *url.URL) bool { return diapos.IDDeEnlace(u) != "" })
	s.AlAbrirURL(func(u *url.URL) {
		if id := diapos.IDDeEnlace(u); id != "" {
			fyne.Do(func() { d.AbrirID(id, "") })
		}
	})
	// La conversión en SOLO LECTURA, de reserva: sin sesión o sin permiso para crear, una
	// presentación se enseña así. Lo que no se sepa leer, o cualquier fallo antes, sigue en
	// la web.
	cargar := func(c context.Context, ap api.Apertura) error {
		imp, err := s.Importar(c, ap)
		if err != nil {
			return err
		}
		return d.AbrirImportado(imp) //nolint:contextcheck // lo que el editor pida después (el asistente) lleva su propio plazo, no el de esta apertura
	}
	// «Abrir un archivo» del menú Archivo y de la portada: el selector del sistema y, con
	// lo elegido, lo de la portada de la web —una presentación NUEVA y editable con lo
	// convertido—; lo de otro editor lo abre el sistema con su app.
	d.AbrirArchivo = &editor.AbrirArchivo{
		Tipo:     editor.Presentacion,
		Importar: s.Importar,
		Svc:      d.Portada().Svc,
		AbrirID:  d.AbrirID,
		Cargar:   cargar,
		Fuera:    ficheros.Abrir,
		AlFallar: func(string, error) {
			d.Avisar(tr.T("import.error.title", nil), tr.T("import.error.description", nil))
		},
	}
	d.Portada().AlAbrirArchivo = d.AbrirArchivo.Abrir
	// Una presentación pulsada en Drive (`sytue-slides:/open-from-drive?file=…`), soltada
	// en la ventana, abierta con «Abrir con» o en los argumentos se abre EDITABLE, como en
	// la web: la de Drive, en el documento anclado a ese fichero; la de la máquina, en una
	// presentación nueva. Si no se puede abrir, se dice.
	s.AlAbrirFichero("slides", d.AbrirArchivo.AbrirApertura)
	s.AceptarFicheros(w, tr.T("import.error.title", nil), tr.T("open.error.description", nil))
	s.Arrancar(context.Background())
	w.ShowAndRun()
}

// abrirJSON abre un JSON del disco y lo guarda en el mismo sitio.
func abrirJSON(d *diapos.App, ruta string) {
	// La ruta la escribe quien lanza el programa en SU equipo: no cruza ninguna frontera
	// de confianza, que es lo que el análisis de gosec no puede saber.
	crudo, err := os.ReadFile(ruta) //nolint:gosec // G703: argumento de la línea de órdenes
	if err != nil {
		// El permiso denegado del sistema de ficheros es el mismo estado que el 403 del
		// servicio: no es un error, es que esta cuenta no puede ver esto.
		if os.IsPermission(err) {
			d.Fallo(editor.SinPermiso)
			return
		}
		d.Fallo(editor.Error)
		return
	}
	d.AbrirContenido(strings.TrimSuffix(filepath.Base(ruta), filepath.Ext(ruta)), string(crudo), true,
		func(modelo string) error {
			return os.WriteFile(ruta, []byte(modelo), 0o600) //nolint:gosec // G703: el mismo fichero de la línea de órdenes
		})
}
