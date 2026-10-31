// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command calcus es Sytue Calcus, la hoja de cálculo, como aplicación propia de
// escritorio y móvil.
//
// Es su propio binario y no una pantalla de otro programa: en nativo cada producto de
// la suite se instala por separado, igual que en la web cada uno tiene su subdominio
// (`docs/CONVENTIONS.md` §13.6). Las demás apps lo invocan por el esquema de URL que
// declara su `FyneApp.toml`.
//
// Monta lo que la web: la portada con las hojas de cálculo (`/calcus/` en la web) y, al
// abrir una, el editor con el cromo compartido y el lienzo de la hoja, que se guarda en
// el servicio. Un fichero de fuera —de Drive o de la máquina— se enseña convertido y en
// solo lectura.
package main

// La portada y la papelera son las de `editor` (portada.go, papelera.go), compartidas por
// los tres editores; lo que las hace de este producto es montarlas aquí con su tipo, como
// las páginas de su app en la web montan las vistas comunes de `@sytue/office`.
//
// paridad-web: apps/frontend/apps/calcus/src/pages/index.astro
// paridad-web: apps/frontend/apps/calcus/src/pages/trash.astro

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"git.sytue.eu/sytue/one/apps/fyne/calcus"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
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

	// El catálogo es el MISMO que el de Docus y el de Diapos: en la web montan el mismo
	// paquete de la ofimática (`@sytue/office`).
	tr, err := i18n.New(s.Idioma(), i18n.Common(), catalogos.Docs())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}

	// El título es el NOMBRE del producto, que es marca y no se traduce; sale de
	// FyneApp.toml, donde ya está declarado una vez para las cinco plataformas.
	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	// La portada y el editor hablan con el servicio con la sesión TRANSVERSAL: sin
	// sesión, la portada lo dice y ofrece entrar; no se abre el navegador por sorpresa.
	c := calcus.NuevaApp(tr.T, tr.Locale(), api.NewDocs(s.URL("docs"), s.Sesion, nil))
	// Lo que copia «Compartir»: la página del documento en la web y la de un enlace
	// público, `/s/<token>` bajo el prefijo de Docs (la vista de solo lectura de la web).
	// La organización activa encabeza el enlace al editor (ADR 0056); el del enlace
	// PÚBLICO va sin org a propósito: es para quien no tiene sesión, como en la web.
	org := s.Sesion.Organizacion
	c.EnlaceDoc = func(id string) string { return api.AppURL(s.Apex(), "sheets", org(), "edit?id="+url.QueryEscape(id)) }
	c.EnlacePublico = func(token string) string { return api.AppURL(s.Apex(), "sheets", "", "s/"+url.PathEscape(token)) }
	c.Directorio = api.NewDirectorio(s.URL("id"), s.Sesion, nil)
	// «Rellenar con IA» va en directo al servicio de IA, por el ingress que el chart abre
	// a las apps nativas; la web pasa por su BFF, que solo reenvía.
	ia := api.NewIA(s.URL("ai"), s.Sesion, nil)
	c.IA = func(ctx context.Context, instruccion, contexto string) (string, error) {
		return ia.Asistir(ctx, api.PeticionIA{Accion: "rellenar", Tipo: "spreadsheet", Instruccion: instruccion, Contexto: contexto})
	}
	c.AlTitulo = func(t string) {
		if t == "" {
			t = a.Metadata().Name
		}
		w.SetTitle(t)
	}
	// La galería de plantillas de «Crear nuevo» y su selector de tema (editor/plantillas.go).
	c.Portada().ConPlantillas = true
	c.Portada().AlEntrar = func() {
		go func() {
			if err := s.Entrar(context.Background()); err == nil {
				fyne.Do(c.Arrancar)
			}
		}()
	}
	// Entrar o salir desde el menú de cuenta de ESTA o de otra app recarga la lista.
	dentro := s.Estado().Dentro()
	s.AlCambiar(func(e sesion.Estado) {
		if e.Dentro() != dentro {
			dentro = e.Dentro()
			c.Arrancar()
		}
	})
	w.SetContent(ui.NuevaBarraSuite(tr, s, iconos.Sheets).ConSinSesionPropia().Montar(c.Objeto()))
	w.Resize(fyne.NewSize(1280, 800))
	c.Arrancar()

	// `sytue-sheets:/edit?id=…` es el documento del servicio, el mismo que abre la web en
	// `/calcus/edit?id=…` (el Hub lo emite desde lo reciente y la búsqueda): se lee y se
	// guarda allí. Es ruta PROPIA: no es la página «no encontrada».
	s.RutaPropia(func(u *url.URL) bool { return calcus.IDDeEnlace(u) != "" })
	s.AlAbrirURL(func(u *url.URL) {
		if id := calcus.IDDeEnlace(u); id != "" {
			go fyne.Do(func() { c.AbrirID(id, "") }) // puede llegar antes de arrancar el bucle
		}
	})
	// La conversión en SOLO LECTURA, de reserva: sin sesión o sin permiso para crear, un
	// fichero se enseña así. Lo que el lienzo no entiende, o cualquier fallo antes, sigue
	// en la web.
	cargar := func(ctx context.Context, ap api.Apertura) error {
		imp, err := s.Importar(ctx, ap)
		if err != nil {
			return err
		}
		if imp.Kind != api.DocSheet {
			return fmt.Errorf("calcus: %q no es una hoja de cálculo (%s)", imp.Title, imp.Kind)
		}
		entendida := make(chan bool, 1)
		fyne.Do(func() { entendida <- c.AbrirConvertido(imp.Title, imp.Content) })
		select {
		case ok := <-entendida:
			if !ok {
				return fmt.Errorf("calcus: %q trae algo que la hoja nativa no sabe enseñar", imp.Title)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// «Abrir un archivo» del menú Archivo y de la portada: el selector del sistema y, con
	// lo elegido, lo de la portada de la web —una hoja NUEVA y editable con lo
	// convertido—; lo de otro editor lo abre el sistema con su app.
	c.PonerAbrir(nil, cargar, ficheros.Abrir)
	c.ImportarAlAbrir(s.Importar)
	// Una hoja pulsada en Drive (`sytue-sheets:/open-from-drive?file=…`), soltada en la
	// ventana, abierta con «Abrir con» o en los argumentos se abre EDITABLE, como en la
	// web: la de Drive, en el documento anclado a ese fichero; la de la máquina, en una
	// hoja nueva. Si no se puede abrir, se dice.
	s.AlAbrirFichero("sheets", c.AbrirApertura)
	s.AceptarFicheros(w, tr.T("import.error.title", nil), tr.T("open.error.description", nil))
	s.Arrancar(context.Background())
	w.ShowAndRun()
}
