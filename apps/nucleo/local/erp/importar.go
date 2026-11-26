// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/banca"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/erp/importar"
)

// maxImportBytes acota el fichero que se importa, como en el servidor.
const maxImportBytes = 8 << 20

// importar — POST /{modelo}/importar — reparte por el CONTENIDO: un extracto Norma 43
// en el banco; un CSV, como lote con simulacro y marcha atrás.
func (s *Servidor) importar(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	cuerpo, err := io.ReadAll(io.LimitReader(r.Body, maxImportBytes+1))
	if err != nil || len(cuerpo) > maxImportBytes {
		fallo(w, http.StatusBadRequest, "bad_request", "The file is too large (8 MiB max).", nil)
		return
	}
	cuerpo = bytes.TrimPrefix(cuerpo, []byte("\xef\xbb\xbf"))
	n43 := (m.Clave == "linea_extracto" || m.Clave == "extracto_bancario") && banca.EsN43(cuerpo)
	if n43 && r.URL.Query().Get("simular") == "1" {
		fallo(w, http.StatusUnprocessableEntity, "importacion_no_simulable", "This file cannot be previewed: only CSV imports can be simulated.", nil)
		return
	}
	if n43 {
		s.importarN43(w, r, cuerpo)
		return
	}
	// Una factura electrónica en XML (UBL, CII o Facturae) entra como factura recibida
	// (facturas.go, con apps/nucleo/erp/efactura).
	if t := bytes.TrimSpace(cuerpo); len(t) > 0 && t[0] == '<' {
		if r.URL.Query().Get("simular") == "1" {
			fallo(w, http.StatusUnprocessableEntity, "importacion_no_simulable", "This file cannot be previewed: only CSV imports can be simulated.", nil)
			return
		}
		s.importarFacturas(w, r, m, cuerpo)
		return
	}
	s.importarCSV(w, r, m, cuerpo)
}

// maxImportFilas acota lo que se procesa de una vez, como en el servidor.
const maxImportFilas = 5000

func (s *Servidor) importarCSV(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo, cuerpo []byte) {
	ctx := r.Context()
	lectura, err := importar.Leer(ctx, cuerpo, m, s.cat.VistaModelo(m, idioma(r)), maxImportFilas,
		func(ctx context.Context, modelo, texto string) (string, error) {
			id, err := s.alm.BuscarPorEtiqueta(ctx, modelo, texto)
			if errors.Is(err, ErrNoEncontrado) {
				return "", importar.ErrSinRegistro
			}
			return id, err
		})
	switch {
	case errors.Is(err, importar.ErrCSV):
		fallo(w, http.StatusUnprocessableEntity, "csv_invalido", "The file could not be read as CSV.", nil)
		return
	case errors.Is(err, importar.ErrSinColumnas):
		fallo(w, http.StatusUnprocessableEntity, "csv_sin_columnas", "No column header matches a field of this model.", nil)
		return
	case err != nil:
		falloDe(w, err)
		return
	}
	simular := r.URL.Query().Get("simular") == "1"
	fichero := strings.TrimSpace(r.URL.Query().Get("fichero"))
	if len(fichero) > 300 {
		fichero = fichero[:300]
	}
	res, err := s.alm.Importar(ctx, m, lectura.Filas, fichero, simular)
	if err != nil {
		falloDe(w, err)
		return
	}
	res.Fallos = append(lectura.Fallos, res.Fallos...)
	sort.SliceStable(res.Fallos, func(i, j int) bool { return res.Fallos[i].Fila < res.Fallos[j].Fila })
	res.Filas = lectura.Leidas
	responder(w, http.StatusOK, struct {
		ResultadoImportacion
		Simulacion bool     `json:"simulacion"`
		Ignoradas  []string `json:"ignoradas"`
	}{res, simular, lectura.Ignoradas})
}

func (s *Servidor) importaciones(w http.ResponseWriter, r *http.Request) {
	lotes, err := s.alm.Importaciones(r.Context())
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"importaciones": lotes})
}

func (s *Servidor) revertirImportacion(w http.ResponseWriter, r *http.Request) {
	res, err := s.alm.Revertir(r.Context(), r.PathValue("id"))
	if errors.Is(err, ErrYaRevertida) {
		fallo(w, http.StatusConflict, "ya_revertida", "That import was already undone.", nil)
		return
	}
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, res)
}
