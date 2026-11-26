// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"encoding/csv"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/exportar"
)

// maxExportar es el tope de filas de una descarga, el de services/syerp.
const maxExportar = 50_000

// exportarCSV — GET /{modelo}/export — la lista en CSV con los filtros de la pantalla
// (`q`, `estado`) o solo las filas de `ids`, con las mismas celdas que el servidor
// (apps/nucleo/erp/exportar).
func (s *Servidor) exportarCSV(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	q := r.URL.Query()
	f := api.ERPFiltro{Q: q.Get("q"), Estado: q.Get("estado"), Limite: 200}
	if f.Estado != "" && !slices.Contains(m.Estados, f.Estado) {
		estadoInvalido(w)
		return
	}
	var ids map[string]bool
	if crudo := q.Get("ids"); crudo != "" {
		partes := strings.Split(crudo, ",")
		if len(partes) > 500 {
			fallo(w, http.StatusBadRequest, "ids_invalidos", "The ids must be record ids separated by commas, at most 500.", nil)
			return
		}
		ids = map[string]bool{}
		for _, id := range partes {
			ids[id] = true
		}
		f.Q, f.Estado = "", ""
	}
	ctx := r.Context()
	l := idioma(r)
	vista := s.cat.VistaModelo(m, l)
	cols := exportar.Columnas(s.cat, vista, l)
	valores := exportar.RotulosDeValor(vista)
	pag, err := s.alm.Listar(ctx, m, f)
	if err != nil {
		falloDe(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+m.Clave+"-"+s.alm.ahora().UTC().Format("2006-01-02")+`.csv"`)
	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write(exportar.Rotulos(cols))
	escritas := 0
	for {
		for _, reg := range pag.Registros {
			if ids != nil && !ids[reg.ID] {
				continue
			}
			if escritas == maxExportar {
				_ = cw.Write([]string{s.cat.Rotulo("aviso.truncado", l) + " " + strconv.Itoa(maxExportar)})
				return
			}
			_ = cw.Write(exportar.Fila(cols, reg.Ref, reg.Estado, reg.Data, pag.Etiquetas, valores))
			escritas++
		}
		if pag.Siguiente == "" {
			return
		}
		f.Cursor = pag.Siguiente
		if pag, err = s.alm.Listar(ctx, m, f); err != nil {
			return
		}
	}
}
