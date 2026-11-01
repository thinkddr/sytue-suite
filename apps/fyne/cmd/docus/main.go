// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command docus abre Sytue Docus en una ventana de verdad.
//
// Es una aplicación PROPIA y no un modo de otra (`docs/CONVENTIONS.md` §13.6): Docus,
// Calcus y Diapos comparten el cromo —`apps/fyne/editor`— y son tres binarios, tres
// `FyneApp.toml` y tres esquemas de URL, como en la web son tres subdominios.
//
// Para VERLA hace falta una máquina con pantalla; el piloto no la tiene. Lo que sí
// corre en cualquier sitio, sin CGO y sin servidor gráfico, es `go test ./editor/...`,
// que pinta lo mismo con el pintor de software y compara los píxeles.
package main

// La portada y la papelera son las de `editor` (portada.go, papelera.go), compartidas por
// los tres editores; lo que las hace de este producto es montarlas aquí con su tipo, como
// las páginas de su app en la web montan las vistas comunes de `@sytue/office`.
//
// paridad-web: apps/frontend/apps/docus/src/pages/index.astro
// paridad-web: apps/frontend/apps/docus/src/pages/trash.astro

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/sesion"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
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

	// El catálogo es el MISMO para Docus, Calcus y Diapos: en la web montan el mismo
	// paquete de la ofimática (`@sytue/office`) y comparten los 2.195 mensajes. Una copia
	// por binario dejaría la primera corrección en dos de los tres.
	tr, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Docs())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El título es el NOMBRE del producto (FyneApp.toml), y con un documento abierto su
	// título, como Calcus y Diapos. Antes decía «Edit — Sytue Docus» también en la portada.
	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	// La portada y el editor hablan con el servicio con la sesión TRANSVERSAL: sin
	// sesión, la portada lo dice y ofrece entrar; no se abre el navegador por sorpresa.
	docs := api.NewDocs(s.URL("docs"), s.Sesion, nil)
	d := docus.NuevaApp(tr.T, tr.Locale(), docs)
	d.Ventana = w                  // «Pantalla completa» de Vista
	d.Reclamos = s.Sesion.Reclamos // quién firma las marcas del control de cambios
	// Lo que copia «Compartir»: la página del documento en la web y la de un enlace
	// público, `/s/<token>` bajo el prefijo de Docs (la vista de solo lectura de la web).
	// La organización activa encabeza el enlace al editor (ADR 0056); el del enlace
	// PÚBLICO va sin org a propósito: es para quien no tiene sesión, como en la web.
	org := s.Sesion.Organizacion
	d.EnlaceDoc = func(id string) string { return api.AppURL(s.Apex(), "docs", org(), "edit?id="+url.QueryEscape(id)) }
	d.EnlacePublico = func(token string) string { return api.AppURL(s.Apex(), "docs", "", "s/"+url.PathEscape(token)) }
	d.Directorio = api.NewDirectorio(s.URL("id"), s.Sesion, nil)
	d.AlTitulo = func(t string) {
		if t == "" {
			t = a.Metadata().Name
		}
		w.SetTitle(t)
	}
	d.Asistir, d.Idioma = api.NewIA(s.URL("ai"), s.Sesion, nil).Asistir, string(s.Idioma()) // el panel «Asistente»
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
	w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Docs).ConSinSesionPropia().Montar(d.Objeto()))
	w.Resize(fyne.NewSize(1280, 800))
	d.Arrancar()

	// `sytue-docs:/docus/edit?id=…` es el documento del servicio, el mismo que abre la web:
	// llega de «Abrir en la aplicación» del editor web, de la búsqueda y de los avisos.
	// Se lee y se guarda allí. Es ruta PROPIA: no es la página «no encontrada».
	s.RutaPropia(func(u *url.URL) bool { return api.IDDeEdicion("docs", u) != "" })
	s.AlAbrirURL(func(u *url.URL) {
		if id := api.IDDeEdicion("docs", u); id != "" {
			go fyne.Do(func() { d.AbrirID(id, "") }) // puede llegar antes de arrancar el bucle
		}
	})

	// La conversión en SOLO LECTURA, de reserva: sin sesión o sin permiso para crear, un
	// documento se enseña así. Si algo falla —sin fichero, un formato que no es de texto—
	// la sesión lo manda a la misma página en la web.
	cargar := func(ctx context.Context, ap api.Apertura) error {
		imp, err := s.Importar(ctx, ap)
		if err != nil {
			return err
		}
		if imp.Kind != api.DocText {
			return fmt.Errorf("docus: %q no es un documento de texto (%s)", imp.Title, imp.Kind)
		}
		fyne.Do(func() {
			d.AbrirConvertido(imp.Title, imp.Content)
			w.SetTitle(imp.Title)
		})
		return nil
	}
	// «Abrir un archivo» del menú Archivo: el selector del sistema y, con lo elegido, lo
	// de la portada de la web —un documento NUEVO y editable con lo convertido—; lo de
	// otro editor lo abre el sistema con su app. El `.txt`, el `.md` y el `.html` no son
	// de ninguna app, pero el `accept` de la portada de la web los admite.
	d.AbrirArchivo = &editor.AbrirArchivo{
		Tipo:     editor.Documento,
		Importar: s.Importar,
		// Un documento nuevo con lo convertido: con los comentarios del fichero.
		ImportarGuardando: s.ImportarGuardando,
		Svc:               docs,
		AbrirID:           d.AbrirID,
		Cargar:            cargar,
		Fuera:             ficheros.Abrir,
		Importa:           []string{"txt", "md", "markdown", "html", "htm"},
		AlFallar: func(string, error) {
			d.Avisar(tr.T("import.error.title", nil), tr.T("import.error.description", nil))
		},
	}
	// Y el mismo, en «Abrir un archivo» de la portada, como en la web.
	d.Portada().AlAbrirArchivo = d.AbrirArchivo.Abrir
	// Un `.docx` pulsado en Drive (`sytue-docs:/open-from-drive?file=…`), soltado en la
	// ventana, abierto con «Abrir con» o en los argumentos se abre EDITABLE, como en la
	// web: el de Drive, en el documento anclado a ese fichero; el de la máquina, en un
	// documento nuevo. Si no se puede abrir, se dice.
	s.AlAbrirFichero("docs", d.AbrirArchivo.AbrirApertura)
	s.AceptarFicheros(w, tr.T("import.error.title", nil), tr.T("open.error.description", nil))
	// Y el texto o el enlace soltado sobre la página entra donde cae, como en la web.
	soltar.AlSoltar(w, func(x soltar.Soltado) { d.Soltar(x.ComoTexto(), x.Posicion) })
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
