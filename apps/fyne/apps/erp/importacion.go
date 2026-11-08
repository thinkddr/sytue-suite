// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Importar con vista previa y deshacer el lote (`lib/importacion.ts`)
//
// Los CSV se mandan primero con `simular=1` —el servicio hace la importación entera
// contra la base y la deshace— y se pregunta con lo que pasaría. Después, el aviso del
// resultado lleva «Deshacer», que revierte los lotes que acaban de entrar. Las facturas
// en XML no entran como lote: ni vista previa ni deshacer. El servicio decide por el
// CONTENIDO: un XML o un Norma 43 con extensión `.csv` contestan a la simulación 422 sin
// escribir nada, y entran sin vista previa. Si la simulación no se puede hacer (red,
// 5xx, o un 4xx que no habla del fichero: 401, 408, 429), no se importa nada y se dice;
// un 4xx del fichero (codigosDelFichero) lo diría igual la importación de verdad y no
// se pregunta por él.

// ficheroLeido es un fichero elegido, ya leído.
type ficheroLeido struct {
	nombre string
	xml    bool
	datos  []byte
	err    error
}

// leerFicheros lee las rutas elegidas. Un fichero que no se puede leer se queda con su
// error y lo cuenta la importación, como un fallo más.
func leerFicheros(rutas []string) []ficheroLeido {
	out := make([]ficheroLeido, 0, len(rutas))
	for _, ruta := range rutas {
		datos, err := os.ReadFile(ruta) //nolint:gosec // G304: la ruta la eligió quien usa la app en el selector del sistema
		out = append(out, ficheroLeido{
			nombre: filepath.Base(ruta), xml: strings.EqualFold(filepath.Ext(ruta), ".xml"), datos: datos, err: err,
		})
	}
	return out
}

// previa es lo que pasaría con todos los CSV juntos (`Previa` de la web).
type previa struct {
	creados, actualizados int
	fallos                []string
}

// simulado es la simulación de un fichero, con su nombre.
type simulado struct {
	nombre string
	res    api.ERPResultadoImportacion
}

// sumarPrevia es `sumarPrevia`: suma las simulaciones y pone una línea por fila que no
// entraría, con el nombre del fichero delante si hay varios.
func sumarPrevia(rs []simulado, fila func(n int, motivo string) string) previa {
	var p previa
	for _, s := range rs {
		p.creados += s.res.Creados
		p.actualizados += s.res.Actualizados
		for _, f := range s.res.Fallos {
			texto := fila(f.Fila, f.Error)
			if len(rs) > 1 {
				texto = s.nombre + " · " + texto
			}
			p.fallos = append(p.fallos, texto)
		}
	}
	return p
}

// merecePreguntar: si no entraría nada, la vista previa no aporta; se importa y se ven
// los fallos en el aviso.
func (p previa) merecePreguntar() bool { return p.creados+p.actualizados > 0 }

// codigosDelFichero son los 4xx que hablan del FICHERO (`CODIGOS_DEL_FICHERO` de la
// web): ilegible, sin columnas o no simulable (422, `csv_invalido`, `csv_sin_columnas`,
// `importacion_no_simulable`), demasiado grande o petición mala (400: el servicio corta
// el cuerpo con `MaxBytesReader`), sin permiso para importar ese modelo (403), demasiado
// grande antes de llegar al servicio (413) o de un tipo que no se admite (415). Un 401
// de sesión caducada, un 408 o un 429 de límite no dicen nada de él.
var codigosDelFichero = []int{
	http.StatusBadRequest, http.StatusForbidden, http.StatusRequestEntityTooLarge,
	http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity,
}

// falloDelFichero es `falloDelFichero` de la web: uno de codigosDelFichero lo diría
// igual la importación de verdad, así que no se pregunta por él. Un corte, un 5xx o un
// 4xx de otra cosa (sesión, límite) no dice nada del fichero: no se importa a ciegas.
func falloDelFichero(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && slices.Contains(codigosDelFichero, e.Status)
}

// simular manda los CSV con `simular=1`. Si una simulación falla por algo que no es del
// fichero (falloDelFichero), devuelve ese error: no se importa a ciegas.
func (p *Pantalla) simular(modelo string, fs []ficheroLeido) (previa, error) {
	var rs []simulado
	for _, f := range fs {
		if f.xml || f.err != nil {
			continue
		}
		ctx, cancelar := p.ctx()
		res, err := p.svc.ImportarCon(ctx, modelo, f.datos, false, api.ERPOpcionesImportacion{Fichero: f.nombre, Simular: true})
		cancelar()
		switch {
		case err == nil:
			rs = append(rs, simulado{f.nombre, res})
		case !falloDelFichero(err):
			return previa{}, err
		}
	}
	return sumarPrevia(rs, func(n int, e string) string {
		return p.t("list.importFailedRow", i18n.Vars{"row": n, "error": e})
	}), nil
}

// textoPrevia es el mensaje del diálogo: lo que pasaría y las tres primeras filas que no
// entrarían, en una línea como en la web.
func (p *Pantalla) textoPrevia(pr previa) string {
	partes := make([]string, 0, 1+min(3, len(pr.fallos)))
	partes = append(partes, p.t("list.importPreview", i18n.Vars{
		"created": pr.creados, "updated": pr.actualizados, "failed": len(pr.fallos),
	}))
	partes = append(partes, pr.fallos[:min(3, len(pr.fallos))]...)
	return strings.Join(partes, " · ")
}

// preguntarImportacion abre la vista previa; sin ventana (o sin nada que preguntar)
// importa directamente, como hace la web cuando nada entraría.
func (p *Pantalla) preguntarImportacion(pr previa, si func()) {
	if p.lienzo == nil || !pr.merecePreguntar() {
		si()
		return
	}
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cuerpo := ui.NuevoParrafo(p.textoPrevia(pr), tema.TextoSM, ui.Normal, "text", v)
	var d *ui.Dialogo
	d = ui.Confirmacion(p.t("list.importPreviewTitle"), p.tr.T("ui.cancel", nil), p.t("list.importConfirm"), false, cuerpo,
		func() { d.Cerrar() },
		func() { d.Cerrar(); si() })
	d.RotuloCerrar = p.tr.T("ui.close", nil)
	d.Mostrar(p.lienzo)
}

// importacion es lo que pasó con todos los ficheros de una importación.
type importacion struct {
	creados, actualizados int
	ignoradas, fallos     []string
	// lotes son los ids de los lotes que entraron: lo que deshace «Deshacer».
	lotes []string
}

// importarLeidos sube los ficheros uno tras otro y suma lo que pasó. Con varios, cada
// fallo lleva delante el nombre de su fichero.
func (p *Pantalla) importarLeidos(modelo string, fs []ficheroLeido) importacion {
	var r importacion
	for _, f := range fs {
		var res api.ERPResultadoImportacion
		err := f.err
		if err == nil {
			ctx, cancelar := p.ctx()
			res, err = p.svc.ImportarCon(ctx, modelo, f.datos, f.xml, api.ERPOpcionesImportacion{Fichero: f.nombre})
			cancelar()
		}
		if err != nil {
			motivo := MensajeDeError(err, p.tr)
			if len(fs) > 1 {
				motivo = f.nombre + ": " + motivo
			}
			r.fallos = append(r.fallos, motivo)
			continue
		}
		r.creados += res.Creados
		r.actualizados += res.Actualizados
		if res.ID != "" && res.Creados+res.Actualizados > 0 {
			r.lotes = append(r.lotes, res.ID)
		}
		for _, c := range res.Ignoradas {
			if !slices.Contains(r.ignoradas, c) {
				r.ignoradas = append(r.ignoradas, c)
			}
		}
		for _, x := range res.Fallos {
			fila := p.t("list.importFailedRow", i18n.Vars{"row": x.Fila, "error": x.Error})
			if len(fs) > 1 {
				fila = f.nombre + " · " + fila
			}
			r.fallos = append(r.fallos, fila)
		}
	}
	return r
}

// avisarImportacion cuenta el resultado en un aviso —con «Deshacer» si entró algún
// lote— y los fallos, fila a fila, en otros (como mucho cinco): quien importa mil filas
// quiere saber cuáles no entraron.
func (p *Pantalla) avisarImportacion(m Modelo, r importacion) {
	tono := ui.AvisoExito
	if len(r.fallos) > 0 {
		tono = ui.AvisoInfo
	}
	desc := ""
	if len(r.ignoradas) > 0 {
		desc = p.t("list.importIgnored", i18n.Vars{"columns": strings.Join(r.ignoradas, ", ")})
	}
	av := ui.NuevoAviso(tono, p.t("list.importResult", i18n.Vars{
		"created": r.creados, "updated": r.actualizados, "failed": len(r.fallos),
	}), desc)
	// Revertir un lote pide `administrar` en CONTABILIDAD (`exigirVerboContable`), no en
	// el módulo del modelo: sin él, «Deshacer» siempre contestaría «sin permiso».
	if len(r.lotes) > 0 && p.PuedeAccion("contabilidad", "administrar") {
		av.Duracion = ui.DuracionDeshacer
		av.RotuloAccion = p.t("action.undo")
		lotes := r.lotes
		av.AlPulsarAccion = func() {
			av.Cerrar()
			p.deshacerImportacion(m, lotes)
		}
	}
	p.avisos.Notificar(av)
	for _, f := range r.fallos[:min(5, len(r.fallos))] {
		p.avisar(ui.AvisoPeligro, f)
	}
}

// deshacerImportacion es `revertirImportaciones` + su aviso: revierte cada lote, sigue
// con los demás si uno falla y, al final, lo dice; lo que alguien tocó después de
// importar no se pisa y se cuenta aparte.
func (p *Pantalla) deshacerImportacion(m Modelo, lotes []string) {
	if p.svc == nil {
		return
	}
	p.fondo(func() {
		var total api.ERPReversion
		var primero error
		for _, id := range lotes {
			ctx, cancelar := p.ctx()
			r, err := p.svc.RevertirImportacion(ctx, id)
			cancelar()
			if err != nil {
				if primero == nil {
					primero = err
				}
				continue
			}
			total.Borrados += r.Borrados
			total.Restaurados += r.Restaurados
			total.Intactos += r.Intactos
		}
		enHilo(func() {
			if primero != nil {
				p.avisos.Notificar(ui.NuevoAviso(ui.AvisoPeligro, p.t("list.importUndoFailed"), MensajeDeError(primero, p.tr)))
			} else {
				desc := ""
				if total.Intactos > 0 {
					desc = p.t("list.importUndoKept", i18n.Vars{"n": total.Intactos})
				}
				p.avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, p.t("list.importUndone", i18n.Vars{
					"deleted": total.Borrados, "restored": total.Restaurados,
				}), desc))
			}
			// Sin cerrar la ficha que haya abierta: para entonces puede ser otra.
			p.recargarModelo(m)
			p.Repintar()
		})
	})
}
