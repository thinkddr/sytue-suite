// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	csverp "git.sytue.eu/sytue/one/apps/nucleo/erp/exportar"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/syerp/internal/config"
)

// filasPorTramo es lo que se le pide al store en cada vuelta del cursor. Es el
// máximo que admite `Listar`: cuantas menos vueltas, menos consultas para la
// misma descarga, y en memoria solo vive el tramo que se está escribiendo.
const filasPorTramo = 200

// maxIDsExportar es cuántas filas marcadas se pueden pedir por su identificador: las
// de varias páginas de la lista. Más no caben en una URL que atraviese un proxy.
const maxIDsExportar = 500

var errIDsExportar = httpx.NewError(http.StatusBadRequest, "ids_invalidos",
	"The ids must be record ids separated by commas, at most 500.")

// idsDe lee `?ids=` (identificadores separados por comas): las filas marcadas de la
// lista. Cada uno tiene que ser un UUID, que es lo que la consulta compara.
func idsDe(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	crudo := r.URL.Query().Get("ids")
	if crudo == "" {
		return nil, true
	}
	ids := strings.Split(crudo, ",")
	if len(ids) > maxIDsExportar {
		httpx.Fail(w, r, errIDsExportar)
		return nil, false
	}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			httpx.Fail(w, r, errIDsExportar)
			return nil, false
		}
	}
	return ids, true
}

// Exportar devuelve la lista del modelo en CSV: las mismas columnas que la vista
// de lista, los mismos filtros (`q`, `estado`, `campo`/`valor`) y los rótulos en
// el idioma de la petición. Con `ids`, solo esas filas: las marcadas en la lista.
//
// SE ESCRIBE CONFORME SE LEE. El cursor va trayendo tramos de 200 filas que se
// vuelcan al ResponseWriter y se olvidan, así que lo que gasta el pod no depende
// del tamaño de la organización. Lo que sí crece con ella es el TIEMPO, y por
// eso hay un tope duro de filas (`EXPORT_MAX_FILAS`): el router le da 30 s a
// cada petición, y una descarga que se pasa de ahí no acaba en un fichero
// grande, acaba en un fichero cortado por la mitad.
//
// La contrapartida de escribir a chorro es que el 200 sale con la primera fila:
// a partir de ese byte un fallo ya no se puede convertir en un error HTTP, solo
// en un fichero corto y una línea de log. Por eso el primer tramo se pide ANTES
// de tocar la respuesta, que es cuando fallan casi todos (base caída, filtro
// imposible, cursor inválido).
func (h *Handlers) Exportar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	f, ok := filtroDe(w, r, m)
	if !ok {
		return
	}
	if f.IDs, ok = idsDe(w, r); !ok {
		return
	}
	f.Limite = filasPorTramo

	idioma := httpx.LanguageFromRequest(r)
	vista := h.cat.VistaModelo(m, idioma)
	cols := csverp.Columnas(h.cat, vista, idioma)
	valores := csverp.RotulosDeValor(vista)
	tope := h.cfg.ExportMaxFilas
	if tope <= 0 {
		tope = config.ExportMaxFilasDefecto
	}

	pag, err := h.store.Listar(r.Context(), f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	escritas := 0
	registro := log.From(r.Context())
	cortado := func(err error) {
		registro.Warn("exportación CSV cortada a media escritura",
			"modelo", m.Clave, "filas", escritas, "error", err)
	}

	// Sin Content-Length: el fichero se va componiendo y no se sabe cuánto va a
	// medir. El nombre lleva modelo y día para que diez descargas no se pisen en
	// la carpeta de quien exporta; `m.Clave` es [a-z0-9_] por el catálogo, así
	// que no puede romper las comillas de la cabecera.
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+m.Clave+"-"+time.Now().UTC().Format(time.DateOnly)+`.csv"`)

	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write(csverp.Rotulos(cols)); err != nil {
		cortado(err)
		return
	}

	for {
		// Los nombres legibles de las relaciones del tramo, en UNA consulta: sin
		// esto, exportar mil pedidos serían mil consultas para poner el nombre
		// del cliente.
		etiquetas, err := h.store.ResolverEtiquetas(r.Context(), idsReferenciados(m, pag.Registros))
		if err != nil {
			cortado(err)
			return
		}
		for i := range pag.Registros {
			if escritas == tope {
				// Se corta, pero NO en silencio: quien abra el fichero tiene que
				// ver que le faltan filas ahí mismo, no descubrirlo al cuadrar
				// los totales. Como el 200 ya salió, el aviso solo puede ir
				// dentro del propio CSV.
				if err := cw.Write([]string{h.cat.Rotulo("aviso.truncado", idioma) + " " + strconv.Itoa(tope)}); err != nil {
					cortado(err)
				}
				return
			}
			if err := cw.Write(csverp.Fila(cols, pag.Registros[i].Ref, pag.Registros[i].Estado, pag.Registros[i].Data, etiquetas, valores)); err != nil {
				cortado(err)
				return
			}
			escritas++
		}
		if pag.Siguiente == "" {
			return
		}
		f.Cursor = pag.Siguiente
		// Se suelta lo escrito antes de ir a por el siguiente tramo: así el
		// navegador va recibiendo fichero mientras la base trabaja.
		cw.Flush()
		siguiente, err := h.store.Listar(r.Context(), f)
		if err != nil {
			cortado(err)
			return
		}
		pag = siguiente
	}
}

// Las celdas las forma apps/nucleo/erp/exportar, que comparte el ERP de Sytue Office.
// `importe`, `centimos` y `texto` siguen aquí con su nombre porque los usan Facturae y UBL.

func importe(v any) string         { return csverp.Importe(v) }
func centimos(v any) (int64, bool) { return csverp.Centimos(v) }
func texto(v any) string           { return csverp.Texto(v) }
