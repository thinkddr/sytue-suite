// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command diapos es Sytue Diapos de Sytue Office: las presentaciones de la suite en este
// equipo y en claro, sin cuenta y sin servidor. Ver `cmd-local/docus`.
//
// Sin «Presentar con Cast» (es emitir por el servicio de Cast) ni compartir.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/diapos"
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
	l, err := local.Abrir(a, "slides")
	if err != nil {
		slog.Error("diapos", "error", err)
		os.Exit(1)
	}
	editor.Personal = l.Personal()
	// En local no hay a quién compartir: «Compartir» no sale, en vez de salir inerte (O-02).
	editor.OcultarCompartirSinGancho = true
	tema.ElegirCromo(tema.CromoPapel)
	idioma := l.Idioma()
	tr, err := l.Traductor(catalogos.Docs())
	if err != nil {
		slog.Error("no se pudo cargar el catálogo de idiomas", "error", err)
		os.Exit(1)
	}
	bib, err := l.Biblioteca("slides")
	if err != nil {
		slog.Error("no se pudo abrir la biblioteca", "error", err)
		os.Exit(1)
	}
	// «Exportar en formatos abiertos» (Ajustes → Copia): cada documento en .pptx.
	l.AlExportarTodo(local.ExportarBiblioteca(bib, "pptx"))
	// La misma biblioteca cumple SlidesData y DocsData: la fachada de Diapos saca de ella
	// la de Docs que usan la portada y «Abrir un archivo».
	slides, err := api.NewPrivateSlides().WithPrivateData(bib)
	if err != nil {
		slog.Error("diapos", "error", err)
		os.Exit(1)
	}
	docs := slides.Docs()

	w := a.NewWindow(a.Metadata().Name)
	w.SetPadded(false)
	d := diapos.NuevaApp(tr.T, idioma, docs, slides)
	d.Asistir = l.Asistir()
	l.AlCambiarIA(func() { d.Asistir = l.Asistir() })
	d.Almacen = l.Personal() // el tamaño de las notas del moderador
	d.AlTitulo = func(t string) {
		if t == "" {
			t = a.Metadata().Name
		}
		w.SetTitle(t)
	}
	d.Portada().ConPlantillas = true
	// «Guardar» (Ctrl/Cmd+S) y «Guardar como…» en un fichero del disco (O-04, O-07).
	ef := enfichero.Nuevo(enfichero.Opciones{Ventana: w, Bib: bib, Idioma: string(idioma), Avisar: d.Avisar})
	d.Fichero = ef.Editor()
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

	cargar := func(ctx context.Context, ap api.Apertura) error {
		imp, err := l.Importar(ctx, ap)
		if err != nil {
			return err
		}
		return d.AbrirImportado(imp) //nolint:contextcheck // la presentación abierta vive lo que la ventana, no la apertura que la trajo
	}
	d.AbrirArchivo = &editor.AbrirArchivo{
		Tipo:     editor.Presentacion,
		Importar: l.Importar,
		Svc:      docs,
		AbrirID:  d.AbrirID,
		Cargar:   cargar,
		Fuera:    ficheros.Abrir,
		// Un fichero de la máquina se ancla a su presentación (O-06). Se abren el nativo,
		// `.diapos`, y el JSON del modelo de antes (O-13), que se guarda como `.diapos`.
		AbrirLocal: ef.AbrirLocal,
		Importa:    []string{"diapos", "json"},
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
	// «Guardar en la nube»: la presentación abierta, en PowerPoint (.pptx).
	l.AlExportar(func() (string, []byte, error) {
		id, titulo := bib.Ultimo()
		if id == "" {
			return "", nil, fmt.Errorf("abre una presentación primero")
		}
		datos, err := bib.ExportDocument(context.Background(), id, "ooxml", false, nil)
		if titulo = strings.TrimSpace(titulo); titulo == "" {
			titulo = "Presentación"
		}
		return titulo + ".pptx", datos, err
	})

	w.SetContent(l.Montar(w, iconos.Slides, d.Objeto()))
	w.Resize(fyne.NewSize(1280, 800))
	d.Arrancar()
	w.ShowAndRun()
}
