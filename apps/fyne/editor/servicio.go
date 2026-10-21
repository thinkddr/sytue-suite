// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"bytes"
	"context"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Servicio son las acciones del menú Archivo y de Colaboración que solo necesitan el
// servicio de Docs —descargar, guardar una versión, el historial y restaurar—, con los
// avisos de la web. Son las mismas para los tres editores; nacieron en Docus
// (`docus.App`) y subieron aquí cuando Calcus las necesitó.
type Servicio struct {
	Svc   *api.Docs
	Plazo time.Duration
	T     func(string, i18n.Vars) string
	// Avisar enseña un aviso en la pila de la ventana; se llama en el hilo de la interfaz.
	Avisar func(tono ui.TonoAviso, titulo string)
	// Lanzar corre la petición fuera del hilo de la interfaz; nil, una rutina.
	Lanzar func(func())
	// Carpeta es dónde se dejan las descargas; nil, la del sistema.
	Carpeta func() (string, error)
}

func (s Servicio) lanzar(f func()) {
	if s.Lanzar != nil {
		s.Lanzar(f)
		return
	}
	go f()
}

func (s Servicio) ctx() (context.Context, context.CancelFunc) {
	p := s.Plazo
	if p <= 0 {
		p = 30 * time.Second
	}
	return context.WithTimeout(context.Background(), p)
}

func (s Servicio) avisar(tono ui.TonoAviso, clave string) {
	if s.Avisar == nil {
		return
	}
	titulo := clave
	if s.T != nil {
		titulo = s.T(clave, nil)
	}
	s.Avisar(tono, titulo)
}

// FormatoImprimir es el «formato» que pide [Contexto.Imprimir] a `Exportar`: el PDF, que
// en vez de guardarse en las descargas va al diálogo de impresión del sistema. Así cada
// producto imprime con su mismo guardado previo, sin repetirlo.
const FormatoImprimir = "imprimir"

// ImprimirDatos es quien imprime; las pruebas lo cambian por un espía.
var ImprimirDatos = ficheros.ImprimirDatos

// AtajoImprimir es Ctrl/Cmd+P para el lienzo que tiene el foco (los atajos de la ventana
// no le llegan mientras lo tiene): dice si era ese atajo, y si lo era imprime.
func (c *Contexto) AtajoImprimir(s *desktop.CustomShortcut) bool {
	if s == nil || s.KeyName != fyne.KeyP || s.Modifier != fyne.KeyModifierShortcutDefault {
		return false
	}
	if c != nil && c.Imprimir != nil {
		c.Imprimir()
	}
	return true
}

// PDFSimple es «PDF accesible» APAGADO en el menú Archivo: la descarga en PDF pide el simple
// en vez del etiquetado (PDF/UA-1). El valor cero es el de la web: accesible.
// ponytail: vive lo que el proceso; la web lo guarda en localStorage. Llevarlo a las
// preferencias de la app cuando haya quien lo eche de menos entre sesiones.
var PDFSimple bool

// Descargar es «Descargar como…»: `antes` guarda lo pendiente —el servicio serializa
// desde SU estado, y un autoguardado en cola exportaría una versión anterior a lo que se
// ve—; luego pide el formato y lo deja en la carpeta de descargas sin pisar nada. `listo`
// va en el hilo de la interfaz, con el fichero o sin él.
func (s Servicio) Descargar(id, titulo string, t Tipo, formato string, antes func() error, listo func()) {
	s.descargarCon(id, titulo, t, formato, false, antes, listo)
}

// DescargarPlantilla comparte guardado, materialización y descarga con Descargar;
// solo cambia template:true y la extensión del fichero.
func (s Servicio) DescargarPlantilla(id, titulo string, t Tipo, formato string, antes func() error, listo func()) {
	s.descargarCon(id, titulo, t, formato, true, antes, listo)
}

func (s Servicio) descargarCon(id, titulo string, t Tipo, formato string, plantilla bool, antes func() error, listo func()) {
	s.lanzar(func() {
		c, cancelar := s.ctx()
		defer cancelar()
		var err error
		if antes != nil {
			err = antes()
		}
		var ruta string
		switch {
		case err != nil:
		case formato == FormatoImprimir:
			var datos []byte
			if datos, err = s.Svc.Export(c, id, "pdf"); err == nil {
				err = ImprimirDatos(api.ExportFilename(titulo, t.Kind(), "pdf"), datos)
			}
		default:
			ruta, err = s.descargar(c, id, titulo, t, formato, plantilla)
		}
		fyne.Do(func() {
			if listo != nil {
				listo()
			}
			if err != nil {
				s.avisar(ui.AvisoPeligro, "doc.msg.exportFailed")
				return
			}
			if formato == FormatoImprimir {
				return // lo que se ve es el diálogo del sistema
			}
			// La web no dice nada: el navegador enseña la descarga. Aquí no hay barra de
			// descargas, y sin aviso no se sabría dónde ha ido a parar.
			if s.Avisar != nil {
				s.Avisar(ui.AvisoExito, filepath.Base(ruta))
			}
		})
	})
}

func (s Servicio) descargar(c context.Context, id, titulo string, t Tipo, formato string, plantilla bool) (string, error) {
	exportar, nombre := s.Svc.Export, api.ExportFilename(titulo, t.Kind(), formato)
	if formato == "pdf" && PDFSimple {
		exportar = func(c context.Context, id, _ string) ([]byte, error) { return s.Svc.ExportPDFSimple(c, id) }
	}
	if plantilla {
		exportar, nombre = s.Svc.ExportTemplate, api.ExportTemplateFilename(titulo, t.Kind(), formato)
	}
	datos, err := exportar(c, id, formato)
	if err != nil {
		return "", err
	}
	carpeta := s.Carpeta
	if carpeta == nil {
		carpeta = ficheros.Carpeta
	}
	dir, err := carpeta()
	if err != nil {
		return "", err
	}
	return ficheros.Guardar(dir, nombre, bytes.NewReader(datos))
}

// GuardarVersion es «Guardar versión» con la etiqueta que se haya escrito; vacía, el
// servicio la guarda sin nombre, como la web con `label || undefined`.
func (s Servicio) GuardarVersion(id, etiqueta string) {
	s.lanzar(func() {
		c, cancelar := s.ctx()
		defer cancelar()
		_, err := s.Svc.CreateVersion(c, id, etiqueta)
		fyne.Do(func() {
			if err != nil {
				s.avisar(ui.AvisoPeligro, "versions.saveError")
				return
			}
			s.avisar(ui.AvisoExito, "versions.saved")
		})
	})
}

// Versiones pide el historial (`loadVersions`); un fallo deja la lista vacía, como en
// la web, que la trata como informativa.
func (s Servicio) Versiones(id string, listo func([]api.DocVersion)) {
	s.lanzar(func() {
		c, cancelar := s.ctx()
		defer cancelar()
		vs, _ := s.Svc.Versions(c, id)
		fyne.Do(func() { listo(vs) })
	})
}

// Actividad pide la auditoría (solo el propietario); un fallo llega a `listo` como error,
// que el desplegable dice, y no como una lista vacía.
func (s Servicio) Actividad(id string, listo func([]api.DocActividad, error)) {
	s.lanzar(func() {
		c, cancelar := s.ctx()
		defer cancelar()
		es, err := s.Svc.Actividad(c, id, 0)
		fyne.Do(func() { listo(es, err) })
	})
}

// RestaurarVersion devuelve el documento a esa versión y, si sale, llama a `luego` para
// volver a leerlo. No destruye nada: el servicio añade las operaciones que llevan hasta
// ella.
func (s Servicio) RestaurarVersion(id string, v api.DocVersion, luego func()) {
	s.lanzar(func() {
		c, cancelar := s.ctx()
		defer cancelar()
		err := s.Svc.RestoreVersion(c, id, v.ID)
		fyne.Do(func() {
			if err != nil {
				s.avisar(ui.AvisoPeligro, "versions.restoreError")
				return
			}
			s.avisar(ui.AvisoExito, "versions.restored")
			if luego != nil {
				luego()
			}
		})
	})
}
