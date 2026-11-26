// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/banca"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	importarcsv "git.sytue.eu/sytue/one/apps/nucleo/erp/importar"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/syerp/internal/permisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

const (
	// maxImportBytes acota el cuerpo. Ocho megas de CSV son decenas de miles de
	// filas: más que eso se parte en varios ficheros, como en cualquier ERP.
	maxImportBytes = 8 << 20
	// maxImportFilas acota lo que se procesa en UNA petición, que tiene 30 s.
	maxImportFilas = 5000
)

var (
	errCSVInvalido = httpx.NewError(http.StatusUnprocessableEntity, "csv_invalido",
		"The file could not be read as CSV.")
	errSinColumnas = httpx.NewError(http.StatusUnprocessableEntity, "csv_sin_columnas",
		"No column header matches a field of this model.")
	// errNoSimulable: solo el CSV sabe simular. Una factura en XML o un extracto
	// Norma 43 —que se reconocen por el CONTENIDO, no por la extensión— se escribían
	// de verdad durante la «vista previa» y otra vez al confirmar.
	errNoSimulable = httpx.NewError(http.StatusUnprocessableEntity, "importacion_no_simulable",
		"This file cannot be previewed: only CSV imports can be simulated.")
)

type falloDeFila struct {
	Fila  int    `json:"fila"`
	Error string `json:"error"`
}

// Importar — POST /api/v1/{modelo}/importar — carga un CSV en el modelo. Si lo
// que llega es XML, es una factura electrónica recibida (ver efactura.go).
//
// ES LA INTEROPERABILIDAD UNIVERSAL: todo ERP exporta CSV, así que esto es lo
// que hace que quien llega de Holded, Sage, SAP o una hoja de cálculo tenga sus
// datos el primer día sin esperar a un conector. Acepta el mismo fichero que
// produce la exportación de este ERP (mismas columnas, mismos rótulos), y casi
// cualquier otro:
//
//   - la cabecera puede ir por clave de campo (`nombre`) o por rótulo en el
//     idioma de la petición (`Nombre`); las columnas que no casan se ignoran y
//     se devuelven, para que se vea qué se quedó fuera;
//   - el separador se deduce de la cabecera (coma, punto y coma o tabulador),
//     porque Excel en español exporta con punto y coma;
//   - los importes admiten «1.234,56» y «1234.56»; las fechas, ISO y dd/mm/aaaa;
//   - una relación admite el identificador o el NOMBRE del registro apuntado.
//
// Con columnaEntrante `externo_id` rellena, la fila se crea o actualiza por ese id
// (importar dos veces no duplica); sin ella, se crea. Una fila mala no aborta el
// resto: se cuenta y se devuelve con su número de línea.
func (h *Handlers) Importar(w http.ResponseWriter, r *http.Request) {
	m := h.modeloDe(w, r)
	if m == nil {
		return
	}
	cuerpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxImportBytes))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file is too large (8 MiB max)."))
		return
	}
	cuerpo = bytes.TrimPrefix(cuerpo, []byte("\xef\xbb\xbf"))
	xml := esXML(cuerpo)
	n43 := !xml && (m.Clave == "linea_extracto" || m.Clave == "extracto_bancario") && banca.EsN43(cuerpo)
	// `simular` solo lo entiende importarCSV: a los otros dos se les niega antes de
	// escribir nada, y la interfaz los importa sin vista previa.
	if (xml || n43) && r.URL.Query().Get("simular") == "1" {
		httpx.Fail(w, r, errNoSimulable)
		return
	}
	if xml {
		h.importarFacturas(w, r, m, cuerpo)
		return
	}
	if n43 {
		h.importarN43(w, r, cuerpo)
		return
	}
	h.importarCSV(w, r, m, cuerpo)
}

// importarCSV lee el fichero y valida fila a fila (apps/nucleo/erp/importar, que comparte
// el ERP de Sytue Office) y escribe el LOTE ENTERO de una vez (store.Importar), que es lo
// que lo deja revertible.
//
// Con `?simular=1` hace exactamente lo mismo contra la base y lo deshace: el
// informe que devuelve es lo que pasaría, comprobado contra las filas y las
// reglas que hay de verdad, no contra una copia en memoria de las reglas que se
// desalinearía a la primera.
func (h *Handlers) importarCSV(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo, cuerpo []byte) {
	idioma := httpx.LanguageFromRequest(r)
	ctx := r.Context()
	lectura, err := importarcsv.Leer(ctx, cuerpo, m, h.cat.VistaModelo(m, idioma), maxImportFilas,
		func(ctx context.Context, modelo, texto string) (string, error) {
			id, err := h.store.BuscarPorEtiqueta(ctx, modelo, texto)
			if errors.Is(err, store.ErrNoEncontrado) {
				return "", importarcsv.ErrSinRegistro
			}
			return id, err
		})
	switch {
	case errors.Is(err, importarcsv.ErrCSV):
		httpx.Fail(w, r, errCSVInvalido)
		return
	case errors.Is(err, importarcsv.ErrSinColumnas):
		httpx.Fail(w, r, errSinColumnas)
		return
	case err != nil:
		httpx.Fail(w, r, err)
		return
	}
	filas := make([]store.FilaImportada, 0, len(lectura.Filas))
	for _, f := range lectura.Filas {
		filas = append(filas, store.FilaImportada{Linea: f.Linea, Data: f.Data, Etiqueta: f.Etiqueta, Externo: f.Externo, Estado: f.Estado})
	}
	previos := []store.FalloDeFila{}
	for _, f := range lectura.Fallos {
		previos = append(previos, store.FalloDeFila{Fila: f.Fila, Error: f.Error})
	}
	leidas, ignoradas := lectura.Leidas, lectura.Ignoradas

	simular := r.URL.Query().Get("simular") == "1"
	var res store.ResultadoImportacion
	if simular {
		res, err = h.store.SimularImportacion(ctx, m, filas, sub(r))
	} else {
		res, err = h.store.Importar(ctx, m, filas, nombreDelFichero(r), sub(r))
	}
	if err != nil {
		httpx.Fail(w, r, errorDeEscritura(err))
		return
	}
	// Los fallos de lectura y validación van con los de escritura, en orden de
	// línea: para quien manda el fichero es una sola lista de qué no entró.
	res.Fallos = append(previos, res.Fallos...)
	sort.Slice(res.Fallos, func(i, j int) bool { return res.Fallos[i].Fila < res.Fallos[j].Fila })
	res.Filas = leidas
	httpx.JSON(w, r, http.StatusOK, struct {
		store.ResultadoImportacion
		Simulacion bool     `json:"simulacion"`
		Ignoradas  []string `json:"ignoradas"`
	}{res, simular, ignoradas})
}

// nombreDelFichero es el nombre que el cliente diga que tiene el fichero. Es
// con lo que quien lo subió reconoce su lote en la lista; si no lo manda, el
// lote sigue existiendo y se identifica por su hora.
func nombreDelFichero(r *http.Request) string {
	n := strings.TrimSpace(r.URL.Query().Get("fichero"))
	if len(n) > 300 {
		n = n[:300]
	}
	return n
}

// Importaciones — GET /api/v1/importaciones — los lotes recientes.
func (h *Handlers) Importaciones(w http.ResponseWriter, r *http.Request) {
	lotes, err := h.store.Importaciones(r.Context(), 50)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Un lote dice qué fichero entró en qué modelo y cuántas filas: de los módulos
	// que quien pide no puede leer, nada (NEG-17).
	visibles := make([]store.LoteImportado, 0, len(lotes))
	for _, l := range lotes {
		if h.accesoRegistro(r.Context(), l.Modelo, permisos.Lectura) {
			visibles = append(visibles, l)
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"importaciones": visibles})
}

// RevertirImportacion — POST /api/v1/importaciones/{id}/revertir — deshace un
// lote entero.
func (h *Handlers) RevertirImportacion(w http.ResponseWriter, r *http.Request) {
	res, err := h.store.Revertir(r.Context(), chi.URLParam(r, "id"), sub(r))
	switch {
	case errors.Is(err, store.ErrNoEncontrado):
		httpx.Fail(w, r, httpx.ErrNotFound)
	case errors.Is(err, store.ErrYaRevertida):
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "ya_revertida",
			"That import was already undone."))
	case err != nil:
		httpx.Fail(w, r, errorDeEscritura(err))
	default:
		httpx.JSON(w, r, http.StatusOK, res)
	}
}
