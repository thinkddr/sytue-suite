// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/erp/catalogo"
	"git.sytue.eu/sytue/one/apps/nucleo/validation"
)

// Las rutas del libro (entrega 2), con las formas y los códigos de error de
// `services/syerp/internal/http/handlers.go` y `cierre.go`.

// sinFin es el «hasta» de un período abierto por arriba.
var sinFin = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

func fechaOpcional(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.DateOnly, s)
}

func (s *Servidor) rutasLibro(h func(string, http.HandlerFunc)) {
	v := "/api/v1"
	h("GET "+v+"/libro", s.libro)
	h("GET "+v+"/balance", s.balance)
	h("POST "+v+"/libro/{id}/anular", s.anularAsiento)
	h("GET "+v+"/contabilidad/cierre", s.cierre)
	h("POST "+v+"/contabilidad/cierre", s.cerrar)
	h("POST "+v+"/contabilidad/reapertura", s.reabrir)
}

func (s *Servidor) asentar(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	if m.Contable == nil {
		fallo(w, http.StatusUnprocessableEntity, "no_contable", "This model does not post to the ledger.", nil)
		return
	}
	a, err := s.alm.Asentar(r.Context(), m, r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusCreated, a)
}

func (s *Servidor) asientosDe(w http.ResponseWriter, r *http.Request, m *catalogo.Modelo) {
	as, err := s.alm.AsientosDe(r.Context(), m.Clave, r.PathValue("id"))
	if err != nil {
		falloDe(w, err)
		return
	}
	if as == nil {
		as = []Asiento{}
	}
	responder(w, http.StatusOK, map[string]any{"asientos": as})
}

// periodoDe lee `desde` y `hasta`; ya ha respondido si no valen.
func periodoDe(w http.ResponseWriter, r *http.Request) (desde, hasta time.Time, ok bool) {
	q := r.URL.Query()
	var err error
	if desde, err = fechaOpcional(q.Get("desde")); err != nil {
		fallo(w, http.StatusBadRequest, "bad_request", "desde: YYYY-MM-DD", nil)
		return desde, hasta, false
	}
	if hasta, err = fechaOpcional(q.Get("hasta")); err != nil {
		fallo(w, http.StatusBadRequest, "bad_request", "hasta: YYYY-MM-DD", nil)
		return desde, hasta, false
	}
	return desde, hasta, true
}

func (s *Servidor) libro(w http.ResponseWriter, r *http.Request) {
	desde, hasta, ok := periodoDe(w, r)
	if !ok {
		return
	}
	limite, _ := strconv.Atoi(r.URL.Query().Get("limite"))
	as, err := s.alm.Libro(r.Context(), desde, hasta, limite)
	if err != nil {
		falloDe(w, err)
		return
	}
	if as == nil {
		as = []Asiento{}
	}
	responder(w, http.StatusOK, map[string]any{"asientos": as})
}

func (s *Servidor) balance(w http.ResponseWriter, r *http.Request) {
	desde, hasta, ok := periodoDe(w, r)
	if !ok {
		return
	}
	if !desde.IsZero() && hasta.IsZero() {
		hasta = sinFin
	}
	saldos, err := s.alm.Balance(r.Context(), desde, hasta)
	if err != nil {
		falloDe(w, err)
		return
	}
	var debe, haber int64
	for _, c := range saldos {
		debe += c.Debe
		haber += c.Haber
	}
	responder(w, http.StatusOK, map[string]any{"cuentas": saldos, "total_debe": debe, "total_haber": haber})
}

func (s *Servidor) anularAsiento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Motivo string `json:"motivo"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	if c.Motivo == "" {
		fallo(w, http.StatusBadRequest, "bad_request", "motivo required", nil)
		return
	}
	if err := s.alm.Anular(r.Context(), r.PathValue("id"), c.Motivo); err != nil {
		falloDe(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) cierre(w http.ResponseWriter, r *http.Request) {
	hasta, err := s.alm.CierreVigente(r.Context())
	if err != nil {
		falloDe(w, err)
		return
	}
	historial, err := s.alm.HistorialDeCierres(r.Context())
	if err != nil {
		falloDe(w, err)
		return
	}
	cerrado := ""
	if hasta != nil {
		cerrado = hasta.Format(time.DateOnly)
	}
	responder(w, http.StatusOK, map[string]any{"cerrado_hasta": cerrado, "historial": historial})
}

func (s *Servidor) cerrar(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Hasta  string `json:"hasta"`
		Motivo string `json:"motivo"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	hasta, err := time.Parse(time.DateOnly, strings.TrimSpace(c.Hasta))
	if err != nil {
		fallo(w, http.StatusBadRequest, "bad_request", "hasta: expected a date as YYYY-MM-DD.", nil)
		return
	}
	if hasta.After(s.alm.ahora()) {
		fallo(w, http.StatusBadRequest, "bad_request", "hasta: you cannot close a period that has not finished yet.", nil)
		return
	}
	if err := s.alm.CerrarPeriodo(r.Context(), hasta, validation.TruncateRunes(strings.TrimSpace(c.Motivo), 1000)); err != nil {
		falloDe(w, err)
		return
	}
	responder(w, http.StatusOK, map[string]any{"cerrado_hasta": hasta.Format(time.DateOnly)})
}

func (s *Servidor) reabrir(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Hasta  string `json:"hasta"`
		Motivo string `json:"motivo"`
	}
	if err := leer(r, &c); err != nil {
		falloDe(w, err)
		return
	}
	motivo := validation.TruncateRunes(strings.TrimSpace(c.Motivo), 1000)
	if motivo == "" {
		fallo(w, http.StatusBadRequest, "bad_request", "motivo: reopening a closed period is exceptional; say why.", nil)
		return
	}
	var hasta *time.Time
	if t := strings.TrimSpace(c.Hasta); t != "" {
		d, err := time.Parse(time.DateOnly, t)
		if err != nil {
			fallo(w, http.StatusBadRequest, "bad_request", "hasta: expected a date as YYYY-MM-DD.", nil)
			return
		}
		hasta = &d
	}
	if err := s.alm.ReabrirPeriodo(r.Context(), hasta, motivo); err != nil {
		falloDe(w, err)
		return
	}
	abierto := ""
	if hasta != nil {
		abierto = hasta.Format(time.DateOnly)
	}
	responder(w, http.StatusOK, map[string]any{"cerrado_hasta": abierto})
}
