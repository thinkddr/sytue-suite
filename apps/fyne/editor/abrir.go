// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
)

// «Nuevo» y «Abrir…» del menú Archivo de los tres editores (Docus, Calcus, Diapos).
//
// En la web no están en el editor: se crea y se abre desde la portada, y el navegador
// pone la ventana nueva. En una app de escritorio el editor ES la ventana, así que el
// menú Archivo los lleva delante, como cualquier procesador de textos. El dueño lo pidió
// así el 2026-09-23.
//
// Lo que hace cada uno NO es un camino nuevo:
//
//   - «Nuevo» crea un documento en blanco del tipo de la app en el servicio
//     (`api.Docs.Create`, lo mismo que «En blanco» de la portada) y lo abre.
//   - «Abrir…» abre el selector de ficheros del SISTEMA (`ficheros.Elegir`) y hace con
//     cada fichero lo que «Abrir un archivo» de la portada de la web
//     (`DocList.onImportPick`): el servicio lo convierte, se crea un documento NUEVO con
//     lo convertido y se abre, EDITABLE. Si el fichero es de OTRO editor —un `.xlsx`
//     elegido en Docus—, se le pasa al sistema para que lo abra con la app que tenga
//     asociada, que es Calcus si está instalada: lo mismo que pulsar el fichero en el
//     gestor de ficheros.

// Clave es la clave del producto de este tipo en la suite (`docs`, `sheets`,
// `slides`): la de `api.AppQueAbre`, los esquemas y el catálogo.
func (t Tipo) Clave() string {
	switch t {
	case Hoja:
		return "sheets"
	case Presentacion:
		return "slides"
	case Documento:
	}
	return "docs"
}

// claveNuevo es la entrada «Nuevo …» del menú para ese tipo.
func claveNuevo(t Tipo) string {
	switch t {
	case Hoja:
		return "file.new.spreadsheet"
	case Presentacion:
		return "file.new.presentation"
	case Documento:
	}
	return "file.new.document"
}

// ErrNoEsDeLaSuite: el fichero elegido no es de ningún editor de la suite (un `.png`, un
// `.zip`), así que no hay app que lo abra.
var ErrNoEsDeLaSuite = errors.New("editor: ese fichero no lo abre ninguna app de la suite")

// AbrirArchivo es lo que «Abrir…» necesita del binario. Se construye una vez en su
// `main` y se pone en cada editor que abre (`Contexto.Abrir = a.Abrir`).
type AbrirArchivo struct {
	Tipo Tipo
	// Elegir abre el selector del sistema y llama a `listo` con las rutas elegidas, en
	// el hilo de la interfaz. Cancelar no llama a nada. Nil es [ElegirDelSistema].
	Elegir func(listo func(rutas []string))
	// Importar (`sesion.Suite.Importar`), Svc y AbrirID traen el fichero como documento
	// nuevo y lo abren (ver arriba). Sin los tres, se enseña por Cargar.
	Importar func(ctx context.Context, ap api.Apertura) (api.Importado, error)
	// ImportarGuardando es Importar con los comentarios del fichero (`sesion.Suite
	// .ImportarGuardando`); solo lo pone Docus. Con él, importar los guarda en el documento
	// nuevo antes de sembrar. Nil es Importar, sin comentarios y sin marks.
	ImportarGuardando func(ctx context.Context, ap api.Apertura) (api.Importado, error)
	Svc               *api.Docs
	AbrirID           func(id, titulo string)
	// Cargar enseña el fichero convertido y en SOLO LECTURA. Es la salida de reserva de
	// [AbrirArchivo.AbrirApertura] cuando no se puede crear el documento (sin sesión, sin
	// permiso para crear).
	Cargar func(ctx context.Context, ap api.Apertura) error
	// Fuera se lo pasa al sistema para que lo abra la app asociada (`ficheros.Abrir`).
	Fuera func(ruta string) error
	// AlFallar dice que un fichero no se ha podido abrir; en el hilo de la interfaz.
	AlFallar func(ruta string, err error)
	// Lanzar corre la conversión fuera del hilo de la interfaz; nil, una rutina.
	Lanzar func(func())
	// Importa son las extensiones (sin punto) que ninguna app reclama —`api.AppQueAbre`
	// no las tiene— y que esta importa igual, porque el `accept` de la portada de la web
	// las admite: `csv` en Calcus; `txt`, `md` o `html` en Docus.
	Importa []string
}

// PlazoAbrir es cuánto se espera a que el servicio convierta el fichero.
const PlazoAbrir = 2 * time.Minute

// Abrir es «Abrir…»: el selector y, con lo elegido, [AbrirArchivo.AbrirRuta].
func (a *AbrirArchivo) Abrir() {
	if a == nil {
		return
	}
	elegir := a.Elegir
	if elegir == nil {
		elegir = ElegirDelSistema
	}
	elegir(func(rutas []string) {
		for _, r := range rutas {
			a.AbrirRuta(r)
		}
	})
}

// AbrirRuta abre UN fichero del disco: el de esta app, por su `cargar`; el de otro
// editor de la suite, en su app; y el que no es de nadie, se dice que no se puede.
func (a *AbrirArchivo) AbrirRuta(ruta string) {
	producto, ok := api.AppQueAbre(filepath.Base(ruta))
	if ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(ruta), ".")); !ok && slices.Contains(a.Importa, ext) {
		producto, ok = a.Tipo.Clave(), true
	}
	switch {
	case !ok:
		a.fallo(ruta, ErrNoEsDeLaSuite)
	case producto != a.Tipo.Clave():
		if a.Fuera == nil {
			a.fallo(ruta, ErrNoEsDeLaSuite)
			return
		}
		if err := a.Fuera(ruta); err != nil {
			a.fallo(ruta, err)
		}
	case a.editable():
		a.lanzar(func() {
			ctx, parar := context.WithTimeout(context.Background(), PlazoAbrir)
			defer parar()
			doc, err := a.importar(ctx, api.Apertura{Local: storage.NewFileURI(ruta).String()}, "")
			fyne.Do(func() {
				if err != nil {
					a.fallo(ruta, err)
					return
				}
				a.AbrirID(doc.ID, doc.Title)
			})
		})
	case a.Cargar != nil:
		a.lanzar(func() {
			ctx, parar := context.WithTimeout(context.Background(), PlazoAbrir)
			defer parar()
			if err := a.Cargar(ctx, api.Apertura{Local: storage.NewFileURI(ruta).String()}); err != nil {
				fyne.Do(func() { a.fallo(ruta, err) })
			}
		})
	}
}

// AbrirApertura es lo que la app registra en `sesion.AlAbrirFichero`: un fichero que llega
// de fuera —«Abrir con» o doble clic en el Finder o el Explorador, soltado en la ventana, en
// los argumentos o pulsado en Drive— se abre EDITABLE, igual que en la web:
//
//   - el de la máquina, como «Abrir un archivo»: un documento nuevo con lo convertido;
//   - el de Drive, como `/open-from-drive`: el documento anclado a ese fichero
//     (`source_ref = drive:<id>`) si ya existe, y si no, uno nuevo con el ancla. Abrirlo
//     cien veces no deja cien copias.
//
// Sin sesión o sin permiso para crear documentos se queda en lo de antes: la conversión en
// solo lectura ([AbrirArchivo.Cargar]). Corre fuera del hilo de la interfaz.
func (a *AbrirArchivo) AbrirApertura(ctx context.Context, ap api.Apertura) error {
	if !a.editable() {
		return a.Cargar(ctx, ap)
	}
	fuente := ""
	if ap.Local == "" && ap.Ref.ID != "" {
		fuente = "drive:" + ap.Ref.ID
		// Ya abierto antes y con contenido: no hay que descargar ni convertir nada. Uno sin
		// ninguna operación es el hueco de una siembra fallida, y se vuelve a sembrar.
		if doc, ok, err := a.Svc.BySource(ctx, fuente); err == nil && ok && doc.Seq > 0 {
			fyne.Do(func() { a.AbrirID(doc.ID, doc.Title) })
			return nil
		}
	}
	doc, err := a.importar(ctx, ap, fuente)
	if err != nil {
		if a.Cargar == nil {
			return err
		}
		slog.Warn("editor: no se ha podido crear el documento; se abre en solo lectura", "error", err)
		return a.Cargar(ctx, ap)
	}
	fyne.Do(func() { a.AbrirID(doc.ID, doc.Title) })
	return nil
}

// editable dice si se puede traer el fichero como documento: hace falta convertir, crear
// y abrir.
func (a *AbrirArchivo) editable() bool {
	return a.Importar != nil && a.Svc != nil && a.AbrirID != nil
}

func (a *AbrirArchivo) lanzar(f func()) {
	if a.Lanzar != nil {
		a.Lanzar(f)
		return
	}
	go f()
}

// importar son los tres pasos de la web: convertir, crear y sembrar. La siembra es la
// primera op del documento —actor propio, contador y reloj a 1—; si falla, es un error:
// un documento vacío es peor que uno honesto, porque se creería que el fichero no traía
// nada y se escribiría encima.
//
// Con `fuente` (`drive:<id>`) el documento se ancla al fichero, salvo una plantilla o un
// libro recortado, que son siempre uno nuevo. Si el anclado ya existía con contenido, se
// abre tal cual: sembrarlo pisaría lo que lleve escrito.
func (a *AbrirArchivo) importar(ctx context.Context, ap api.Apertura, fuente string) (api.Document, error) {
	importar := a.Importar
	if a.ImportarGuardando != nil {
		importar = a.ImportarGuardando
	}
	imp, err := importar(ctx, ap)
	if err != nil {
		return api.Document{}, err
	}
	if imp.Kind != a.Tipo.Kind() {
		return api.Document{}, fmt.Errorf("editor: lo convertido es %s y esta app abre %s", imp.Kind, a.Tipo.Kind())
	}
	if imp.Template || imp.Truncated {
		fuente = ""
	}
	doc, creado, err := a.Svc.CreateDesde(ctx, imp.Kind, imp.Title, fuente)
	if err != nil {
		return api.Document{}, err
	}
	if !creado && doc.Seq > 0 {
		return doc, nil
	}
	// Los comentarios del fichero se guardan ANTES de sembrar: el contenido lleva el ancla de
	// los que quedaron guardados y de ningún otro. Si no se pueden guardar, el documento se abre
	// sin comentarios y sin marks (un mark sin comentario no se puede quitar).
	var anclados []string
	if len(imp.Comments) > 0 {
		var errCom error
		if anclados, errCom = a.Svc.ImportComments(ctx, doc.ID, imp.Comments); errCom != nil {
			slog.Warn("editor: los comentarios del fichero no se han guardado", "error", errCom)
			anclados = nil
		}
	}
	imp.Content = imp.ContenidoConMarks(anclados)
	if imp.Content != "" {
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		dot := api.DocDot{Actor: "fyne-" + hex.EncodeToString(b), Counter: 1}
		if _, err := a.Svc.SaveContent(ctx, doc.ID, a.Tipo.ClaveContenido(), imp.Content, dot, 1, false); err != nil {
			return api.Document{}, err
		}
	}
	return doc, nil
}

func (a *AbrirArchivo) fallo(ruta string, err error) {
	if a.AlFallar != nil {
		a.AlFallar(ruta, err)
	}
}

// ElegirDelSistema es el `Elegir` de siempre: el selector de ficheros DEL SISTEMA —el de
// `<input type="file">`—, que bloquea y por eso va aparte, y lo elegido a `listo` en el
// hilo de la interfaz. Cancelar no llama a nada.
//
// ponytail: sin selector del sistema (un Linux sin zenity ni kdialog, el móvil) no se
// abre nada y queda soltar el fichero en la ventana, que va por el mismo camino.
func ElegirDelSistema(listo func(rutas []string)) {
	go func() {
		rutas, err := ficheros.Elegir()
		if err != nil {
			if !errors.Is(err, ficheros.ErrNoDisponible) {
				slog.Warn("editor: el selector de ficheros ha fallado", "error", err)
			}
			return
		}
		if len(rutas) > 0 {
			fyne.Do(func() { listo(rutas) })
		}
	}()
}
