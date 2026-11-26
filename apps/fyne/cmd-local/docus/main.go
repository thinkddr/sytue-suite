// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command docus es Sytue Docus de Sytue Office: el mismo editor de la suite, con sus
// documentos en este equipo y en claro, sin cuenta y sin servidor.
//
// Comparado con `cmd/docus`: no hay `sesion.Abrir` (ni OIDC ni llavero de sesión), ni
// `cifrado.NewPrivateApplication` (ni Matrix ni dispositivo verificado). La fachada
// `api.Docs` va enchufada a `local/biblioteca` en vez de al repositorio cifrado; abrir un
// fichero lo convierte en el dispositivo; compartir, coedición, directorio y enlaces web
// van a nil, y la pantalla oculta lo que dependa de ellos.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/docus"
	"git.sytue.eu/sytue/one/apps/fyne/editor/enfichero"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/soltar"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "docs")
	if err != nil {
		slog.Error("docus", "error", err)
		os.Exit(1)
	}
	editor.Personal = l.Personal()
	// En local no hay a quién compartir: «Compartir» no sale, en vez de salir inerte (O-02).
	editor.OcultarCompartirSinGancho = true
	tema.ElegirCromo(tema.CromoPapel)
	tr, err := l.Traductor(catalogos.Docs())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}
	bib, err := l.Biblioteca("docs")
	if err != nil {
		slog.Error("no se pudo abrir la biblioteca", "error", err)
		os.Exit(1)
	}
	// «Exportar en formatos abiertos» (Ajustes → Copia): cada documento en .docx.
	l.AlExportarTodo(local.ExportarBiblioteca(bib, "docx"))
	docs, err := api.NewPrivateDocs().WithPrivateData(bib)
	if err != nil {
		slog.Error("docus", "error", err)
		os.Exit(1)
	}

	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	d := docus.NuevaApp(tr.T, tr.Locale(), docs)
	d.Ventana = w
	d.AlTitulo = func(t string) {
		if t == "" {
			t = a.Metadata().Name
		}
		w.SetTitle(t)
	}
	d.Asistir, d.Idioma = l.Asistir(), string(l.Idioma())
	l.AlCambiarIA(func() { d.Asistir = l.Asistir() })
	d.Portada().ConPlantillas = true
	// «Guardar» (Ctrl/Cmd+S) y «Guardar como…» en un fichero del disco (O-04, O-07).
	ef := enfichero.Nuevo(enfichero.Opciones{Ventana: w, Bib: bib, Idioma: string(l.Idioma()), Avisar: d.Avisar})
	d.Fichero = ef.Editor()

	cargar := func(ctx context.Context, ap api.Apertura) error {
		imp, err := l.Importar(ctx, ap)
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
	d.AbrirArchivo = &editor.AbrirArchivo{
		Tipo:              editor.Documento,
		Importar:          l.Importar,
		ImportarGuardando: l.ImportarGuardando,
		Svc:               docs,
		AbrirID:           d.AbrirID,
		Cargar:            cargar,
		Fuera:             ficheros.Abrir,
		// Un fichero de la máquina se ancla a su documento (O-06): abrirlo otra vez da el
		// mismo, y se guarda de vuelta. El `.docus` es el formato nativo (O-05).
		AbrirLocal: ef.AbrirLocal,
		Importa:    []string{"txt", "md", "markdown", "html", "htm", "docus"},
		AlFallar: func(string, error) {
			d.Avisar(tr.T("import.error.title", nil), tr.T("import.error.description", nil))
		},
	}
	d.Portada().AlAbrirArchivo = d.AbrirArchivo.Abrir
	l.AlAbrirFichero(d.AbrirArchivo.AbrirApertura, func(error) {
		d.Avisar(tr.T("import.error.title", nil), tr.T("open.error.description", nil))
	})
	l.AceptarFicheros(w)
	// Un documento que llega por enlace (el Hub: `?clase=documento&id=…`).
	l.AlAbrirElemento(func(_, id string) { d.AbrirID(id, "") })
	soltar.AlSoltar(w, func(x soltar.Soltado) { d.Soltar(x.ComoTexto(), x.Posicion) })
	// «Guardar en la nube»: el documento abierto, en Word (.docx).
	l.AlExportar(func() (string, []byte, error) {
		id, titulo := bib.Ultimo()
		if id == "" {
			return "", nil, fmt.Errorf("abre un documento primero")
		}
		datos, err := bib.ExportDocument(context.Background(), id, "ooxml", false, nil)
		return nombreDe(titulo, ".docx"), datos, err
	})

	w.SetContent(l.Montar(w, iconos.Docs, d.Objeto()))
	w.Resize(fyne.NewSize(1280, 800))
	d.Arrancar()
	w.ShowAndRun()
}

func nombreDe(titulo, ext string) string {
	titulo = strings.TrimSpace(titulo)
	if titulo == "" {
		titulo = "Documento"
	}
	return titulo + ext
}
