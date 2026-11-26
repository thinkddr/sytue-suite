// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package enfichero es la pieza de Sytue Office que une los editores (Docus, Calcus,
// Diapos) con los ficheros del disco (PLAN-EDICION-LOCAL O-04 y O-06 a O-08): abrir un
// fichero lo ancla a su documento de la biblioteca, «Guardar» (Ctrl/Cmd+S) lo escribe de
// vuelta en su sitio y «Guardar como…» elige otro con el selector del sistema.
//
// Las reglas están en `local/biblioteca` (fichero.go); aquí solo las preguntas: la
// primera vez que se guarda encima de un formato ajeno se avisa de que es una conversión
// y de la copia del original; si el fichero cambió por fuera, se pregunta antes de
// pisarlo; y lo que no se puede guardar de vuelta (recortado, plantilla, formato que no
// se escribe) dice por qué y lleva a «Guardar como…».
//
// Solo lo usan los binarios de `cmd-local`; la edición conectada no lo enlaza.
package enfichero

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/local"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// Los textos propios: la web no tiene estas pantallas (§5.14). Español e inglés aquí; los
// otros veinticuatro idiomas, en textos_idiomas.go.
var textos = map[string][2]string{
	"guardar":             {"Guardar", "Save"},
	"guardarComo":         {"Guardar como…", "Save as…"},
	"tituloSelector":      {"Guardar como", "Save as"},
	"cancelar":            {"Cancelar", "Cancel"},
	"guardado":            {"Guardado en «%s»", "Saved to “%s”"},
	"noGuardado":          {"No se ha podido guardar el fichero", "The file could not be saved"},
	"convertirTitulo":     {"Guardar en formato %s", "Save in %s format"},
	"convertirTexto":      {"«%s» no es un fichero de Sytue Office: guardar en él es convertir el documento a %s, y lo que ese formato no tenga se puede perder. La primera vez se guarda al lado una copia del original, «%s».", "“%s” is not a Sytue Office file: saving to it converts the document to %s, and anything that format lacks may be lost. The first time, a copy of the original is kept next to it, “%s”."},
	"convertirVerbo":      {"Convertir y guardar", "Convert and save"},
	"cambioTitulo":        {"El fichero ha cambiado", "The file has changed"},
	"cambioTexto":         {"«%s» se ha cambiado fuera de la app desde que lo abriste o lo guardaste. Si lo sobrescribes, esos cambios se pierden; con «Guardar como…» los conservas.", "“%s” was changed outside the app since you opened or saved it. If you overwrite it, those changes are lost; use “Save as…” to keep them."},
	"sobrescribir":        {"Sobrescribir", "Overwrite"},
	"bloqueadoTitulo":     {"No se puede guardar en el original", "Can’t save to the original"},
	"bloqueadoRecortado":  {"El fichero era demasiado grande y se abrió recortado: guardar encima perdería lo que no se cargó. Guárdalo con otro nombre.", "The file was too large and opened truncated: saving over it would lose what was not loaded. Save it under another name."},
	"bloqueadoPlantilla":  {"Es una plantilla: se abrió como un documento nuevo. Guárdalo con un nombre.", "It is a template: it opened as a new document. Save it under a name."},
	"bloqueadoSinVuelta":  {"Ese formato se puede abrir pero no escribir. Guárdalo en otro formato, por ejemplo «%s».", "That format can be opened but not written. Save it in another format, for example “%s”."},
	"cambioFueraAlAbrir":  {"«%s» ha cambiado fuera de la app y aquí hay cambios que no están en el fichero: se abre lo de la app.", "“%s” changed outside the app and there are changes here that are not in the file: the app’s version is opened."},
	"recargadoAlAbrir":    {"«%s» había cambiado: está al día, y lo de antes queda en el historial de versiones.", "“%s” had changed: it is up to date, and the previous content is in the version history."},
	"sinSelectorGuardado": {"Guardado en tu carpeta de descargas: «%s»", "Saved to your downloads folder: “%s”"},
}

// Opciones es lo que la pieza necesita del binario.
type Opciones struct {
	Ventana fyne.Window
	Bib     *biblioteca.Biblioteca
	// Idioma es el de la app («es», «en»…): los textos propios van en ese idioma, o en
	// inglés si no lo tienen.
	Idioma string
	// Avisar enseña un fallo (`docus.App.Avisar`…), en el hilo de la interfaz.
	Avisar func(titulo, texto string)
	// Plazo para leer y escribir un fichero; 0, dos minutos.
	Plazo time.Duration
}

// Enfichero son «Guardar», «Guardar como…» y abrir anclado, para un editor.
type Enfichero struct {
	o Opciones
}

// Nuevo prepara la pieza.
func Nuevo(o Opciones) *Enfichero {
	if o.Plazo == 0 {
		o.Plazo = 2 * time.Minute
	}
	return &Enfichero{o: o}
}

var tabla = i18n.Propios{Base: textos, Traducciones: traducciones}

func (e *Enfichero) t(clave string, args ...any) string {
	s := tabla.En(i18n.Locale(e.o.Idioma), clave)
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

// Editor es el gancho para `docus.App.Fichero` y sus pares.
func (e *Enfichero) Editor() *editor.Fichero {
	return &editor.Fichero{
		Guardar: e.Guardar, GuardarComo: e.GuardarComo,
		RotuloGuardar: e.t("guardar"), RotuloGuardarComo: e.t("guardarComo"),
	}
}

// Extensiones son las de «Guardar como…» de ese tipo: la nativa primero, que es la que
// no pierde nada, y después las que se saben escribir.
func Extensiones(kind api.DocKind) []string {
	switch kind {
	case api.DocSheet:
		return []string{"calcus", "xlsx", "ods"}
	case api.DocSlides:
		return []string{"diapos", "pptx", "odp"}
	case api.DocText:
	}
	return []string{"docus", "docx", "odt", "md", "html"}
}

// AbrirLocal es `editor.AbrirArchivo.AbrirLocal`: un fichero de la máquina se trae
// anclado a su documento. Lo que no es una ruta (un `content://`) se importa como siempre.
func (e *Enfichero) AbrirLocal(ctx context.Context, ap api.Apertura) (api.Document, error) {
	ruta, ok := biblioteca.RutaDe(ap.Local)
	if !ok {
		return api.Document{}, editor.ErrSeguirImportando
	}
	a, err := e.o.Bib.AbrirFichero(ctx, ruta, true)
	if err != nil {
		return api.Document{}, err
	}
	nombre := filepath.Base(ruta)
	switch {
	case a.CambioFuera:
		fyne.Do(func() { local.Info(e.o.Ventana, e.t("cambioTitulo"), e.t("cambioFueraAlAbrir", nombre)) })
	case a.Recargado:
		fyne.Do(func() { local.Info(e.o.Ventana, e.t("cambioTitulo"), e.t("recargadoAlAbrir", nombre)) })
	}
	return a.Documento, nil
}

// lanzar corre f fuera del hilo de la interfaz, con plazo.
func (e *Enfichero) lanzar(f func(ctx context.Context)) {
	go func() {
		ctx, parar := context.WithTimeout(context.Background(), e.o.Plazo)
		defer parar()
		f(ctx)
	}()
}

func (e *Enfichero) fallo(err error) {
	slog.Warn("enfichero: no se pudo guardar", "error", err)
	fyne.Do(func() {
		if e.o.Avisar != nil {
			e.o.Avisar(e.t("noGuardado"), err.Error())
		}
	})
}

func notificar(ctx *editor.Contexto, texto string) {
	fyne.Do(func() {
		if ctx != nil && ctx.Notificar != nil {
			ctx.Notificar(ui.AvisoExito, texto)
		}
	})
}

// Guardar es «Guardar» y Ctrl/Cmd+S: en el fichero del documento si lo tiene, y si no,
// «Guardar como…».
func (e *Enfichero) Guardar(ctx *editor.Contexto, id string, antes func() error) {
	e.lanzar(func(c context.Context) {
		if antes != nil {
			if err := antes(); err != nil {
				e.fallo(err)
				return
			}
		}
		if _, ok := e.o.Bib.EstadoFichero(id); !ok {
			fyne.Do(func() { e.GuardarComo(ctx, id, nil) }) //nolint:contextcheck // «Guardar como…» es otra operación, con su propio plazo (lanzar)
			return
		}
		e.guardarEn(c, ctx, id, false)
	})
}

// guardarEn escribe en el fichero de origen y contesta a lo que la biblioteca diga.
// Fuera del hilo de la interfaz.
//
//nolint:contextcheck // lo que la persona confirma en los diálogos es otro guardado, con su propio plazo (lanzar)
func (e *Enfichero) guardarEn(c context.Context, ctx *editor.Contexto, id string, forzar bool) {
	f, err := e.o.Bib.GuardarEnFichero(c, id, forzar)
	nombre := filepath.Base(f.Ruta)
	switch {
	case err == nil:
		notificar(ctx, e.t("guardado", nombre))
	case errors.Is(err, biblioteca.ErrConfirmarConversion):
		ext := strings.ToUpper(strings.TrimPrefix(filepath.Ext(nombre), "."))
		copia := strings.TrimSuffix(nombre, filepath.Ext(nombre)) + " (original)" + filepath.Ext(nombre)
		fyne.Do(func() {
			local.Confirmar(e.o.Ventana, e.t("convertirTitulo", ext), e.t("convertirTexto", nombre, ext, copia),
				e.t("cancelar"), e.t("convertirVerbo"), false, func() {
					e.lanzar(func(c context.Context) {
						if err := e.o.Bib.AceptarConversion(id); err != nil {
							e.fallo(err)
							return
						}
						e.guardarEn(c, ctx, id, false)
					})
				})
		})
	case errors.Is(err, biblioteca.ErrCambioFuera):
		fyne.Do(func() {
			local.Confirmar(e.o.Ventana, e.t("cambioTitulo"), e.t("cambioTexto", nombre),
				e.t("cancelar"), e.t("sobrescribir"), true, func() {
					e.lanzar(func(c context.Context) { e.guardarEn(c, ctx, id, true) })
				})
		})
	case errors.Is(err, biblioteca.ErrGuardadoBloqueado):
		motivo := e.t("bloqueadoSinVuelta", "."+Extensiones(e.o.Bib.Tipo())[0])
		switch f.Bloqueo {
		case biblioteca.BloqueoRecortado:
			motivo = e.t("bloqueadoRecortado")
		case biblioteca.BloqueoPlantilla:
			motivo = e.t("bloqueadoPlantilla")
		}
		fyne.Do(func() {
			local.Confirmar(e.o.Ventana, e.t("bloqueadoTitulo"), motivo, e.t("cancelar"), e.t("guardarComo"), false,
				func() { e.GuardarComo(ctx, id, nil) })
		})
	default:
		e.fallo(err)
	}
}

// GuardarComo es «Guardar como…»: el selector de guardado del sistema y, con lo elegido,
// el documento escrito ahí y anclado a ese fichero. Sin selector (el móvil sin el del
// toolkit), va a la carpeta de descargas con el formato nativo.
func (e *Enfichero) GuardarComo(ctx *editor.Contexto, id string, antes func() error) {
	titulo := ""
	if ctx != nil {
		titulo = ctx.Titulo
	}
	e.lanzar(func(c context.Context) {
		if antes != nil {
			if err := antes(); err != nil {
				e.fallo(err)
				return
			}
		}
		exts := Extensiones(e.o.Bib.Tipo())
		nombre := strings.TrimSpace(titulo)
		if f, ok := e.o.Bib.EstadoFichero(id); ok && nombre == "" {
			nombre = strings.TrimSuffix(filepath.Base(f.Ruta), filepath.Ext(f.Ruta))
		}
		ruta, err := ficheros.GuardarComo(e.t("tituloSelector"), nombre, exts...) //nolint:contextcheck // el selector del sistema espera a la persona: no lo corta el plazo del guardado
		sinSelector := errors.Is(err, ficheros.ErrNoDisponible)
		if sinSelector {
			ruta, err = enDescargas(nombre, model.ExtensionNativa(model.Kind(e.o.Bib.Tipo())))
		}
		if err != nil {
			e.fallo(err)
			return
		}
		if ruta == "" {
			return // cancelado
		}
		if _, err := e.o.Bib.GuardarComo(c, id, ruta); err != nil {
			e.fallo(err)
			return
		}
		clave := "guardado"
		if sinSelector {
			clave = "sinSelectorGuardado"
		}
		notificar(ctx, e.t(clave, filepath.Base(ruta)))
	})
}

// enDescargas es una ruta libre en la carpeta de descargas para ese nombre y extensión.
func enDescargas(nombre, ext string) (string, error) {
	dir, err := ficheros.Carpeta()
	if err != nil {
		return "", err
	}
	if nombre == "" {
		nombre = "Documento"
	}
	nombre = strings.NewReplacer("/", "-", "\\", "-").Replace(nombre)
	for i := 1; i < 1000; i++ {
		n := nombre
		if i > 1 {
			n = fmt.Sprintf("%s (%d)", nombre, i)
		}
		p := filepath.Join(dir, n+ext)
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return p, nil
		}
	}
	return "", fmt.Errorf("enfichero: no hay nombre libre para %q", nombre)
}
