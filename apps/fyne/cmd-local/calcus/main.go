// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command calcus es Sytue Calcus de Sytue Office: la hoja de cálculo de la suite con sus
// libros en este equipo y en claro, sin cuenta y sin servidor. Ver `cmd-local/docus`.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/calcus"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/editor/enfichero"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

func main() {
	a := local.NuevaApp("")
	l, err := local.Abrir(a, "sheets")
	if err != nil {
		slog.Error("calcus", "error", err)
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
	bib, err := l.Biblioteca("sheets")
	if err != nil {
		slog.Error("no se pudo abrir la biblioteca", "error", err)
		os.Exit(1)
	}
	// «Exportar en formatos abiertos» (Ajustes → Copia): cada documento en .xlsx.
	l.AlExportarTodo(local.ExportarBiblioteca(bib, "xlsx"))
	docs, err := api.NewPrivateDocs().WithPrivateData(bib)
	if err != nil {
		slog.Error("calcus", "error", err)
		os.Exit(1)
	}

	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	c := calcus.NuevaApp(tr.T, tr.Locale(), docs)
	ponerIA := func() {
		c.IA = nil
		if asistir := l.Asistir(); asistir != nil {
			// «Rellenar con IA», con el proveedor que la persona haya elegido.
			c.IA = func(ctx context.Context, instruccion, contexto string) (string, error) {
				return asistir(ctx, api.PeticionIA{Accion: "rellenar", Tipo: "spreadsheet", Instruccion: instruccion, Contexto: contexto, Idioma: string(l.Idioma())})
			}
		}
	}
	ponerIA()
	l.AlCambiarIA(ponerIA)
	c.AlTitulo = func(t string) {
		if t == "" {
			t = a.Metadata().Name
		}
		w.SetTitle(t)
	}
	c.Portada().ConPlantillas = true
	// «Guardar» (Ctrl/Cmd+S) y «Guardar como…» en un fichero del disco (O-04, O-07).
	ef := enfichero.Nuevo(enfichero.Opciones{Ventana: w, Bib: bib, Idioma: string(l.Idioma()), Avisar: c.Avisar})
	c.Fichero = ef.Editor()

	cargar := func(ctx context.Context, ap api.Apertura) error {
		imp, err := l.Importar(ctx, ap)
		if err != nil {
			return err
		}
		if imp.Kind != api.DocSheet {
			return fmt.Errorf("calcus: %q no es una hoja de cálculo (%s)", imp.Title, imp.Kind)
		}
		entendida := make(chan bool, 1)
		fyne.Do(func() { entendida <- c.AbrirConvertido(imp.Title, imp.Content) }) //nolint:contextcheck // la hoja abierta vive lo que la ventana, no la apertura que la trajo
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
	c.PonerAbrir(nil, cargar, ficheros.Abrir)
	c.ImportarAlAbrir(l.Importar)
	// Un fichero de la máquina se ancla a su libro (O-06); `.calcus` es el nativo (O-05).
	c.AbrirLocalCon(ef.AbrirLocal, "calcus")
	l.AlAbrirFichero(c.AbrirApertura, func(error) {
		c.Avisar(tr.T("import.error.title", nil), tr.T("open.error.description", nil))
	})
	l.AceptarFicheros(w)
	// Un documento que llega por enlace (el Hub: `?clase=documento&id=…`).
	l.AlAbrirElemento(func(_, id string) { c.AbrirID(id, "") })
	// «Guardar en la nube»: el libro abierto, en Excel (.xlsx).
	l.AlExportar(func() (string, []byte, error) {
		id, titulo := bib.Ultimo()
		if id == "" {
			return "", nil, fmt.Errorf("abre una hoja primero")
		}
		datos, err := bib.ExportDocument(context.Background(), id, "ooxml", false, nil)
		return nombreDe(titulo, "Hoja", ".xlsx"), datos, err
	})

	w.SetContent(l.Montar(w, iconos.Sheets, c.Objeto()))
	w.Resize(fyne.NewSize(1280, 800))
	c.Arrancar()
	w.ShowAndRun()
}

func nombreDe(titulo, defecto, ext string) string {
	if titulo = strings.TrimSpace(titulo); titulo == "" {
		titulo = defecto
	}
	return titulo + ext
}
